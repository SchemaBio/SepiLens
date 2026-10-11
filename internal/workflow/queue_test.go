package workflow

import (
	"context"
	"database/sql"
	"errors"
	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/jobs"
	"github.com/pzweuj/sepilens/internal/secure"
	"github.com/pzweuj/sepilens/internal/storage/sqlite"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testQueue(t *testing.T) (*Queue, *jobs.EncryptedStore) {
	t.Helper()
	db, e := sqlite.Open(filepath.Join(t.TempDir(), "queue.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	c, e := secure.New("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	if e != nil {
		t.Fatal(e)
	}
	return &Queue{DB: db, Cipher: c, Evidence: evidence.NewEncryptedStore(db, c)}, jobs.NewEncryptedStore(db, c)
}
func submitTask(t *testing.T, q *Queue) jobs.Job {
	t.Helper()
	j, _, e := q.Submit(context.Background(), "tenant", "acmg_amp_2015", "simulation", "", "digest", []byte(`{"patient":"sensitive-marker"}`))
	if e != nil {
		t.Fatal(e)
	}
	return j
}
func TestLeaseRecoveryDoesNotRepeatUncertainCalls(t *testing.T) {
	q, store := testQueue(t)
	ctx := context.Background()
	j := submitTask(t, q)
	task, owner, e := q.claim(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if task.JobID != j.JobID || owner == "" {
		t.Fatal("not claimed")
	}
	if _, e = q.DB.Exec(`UPDATE work_inputs SET lease_until='2000-01-01',external_started=0 WHERE job_id=?`, j.JobID); e != nil {
		t.Fatal(e)
	}
	task, next, e := q.claim(ctx)
	if e != nil || next == owner || task.JobID != j.JobID {
		t.Fatal("safe task not recovered", e)
	}
	if _, e = q.DB.Exec(`UPDATE work_inputs SET lease_until='2000-01-01',external_started=1 WHERE job_id=?`, j.JobID); e != nil {
		t.Fatal(e)
	}
	if _, _, e = q.claim(ctx); !errors.Is(e, sql.ErrNoRows) {
		t.Fatal("uncertain task replayed", e)
	}
	out, e := store.GetJob(ctx, "tenant", j.JobID)
	if e != nil || out.Status != jobs.StatusFailed || out.Stage != "interrupted" {
		t.Fatal("interruption rolled back", out, e)
	}
	events, e := store.GetEvents(ctx, j.JobID, 0)
	if e != nil || events[len(events)-1].Stage != "interrupted" {
		t.Fatal("missing recovery event", e)
	}
}
func TestCancellationWinsCompletionRace(t *testing.T) {
	q, store := testQueue(t)
	ctx := context.Background()
	j := submitTask(t, q)
	task, owner, e := q.claim(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = store.CancelJob(ctx, "tenant", j.JobID); e != nil {
		t.Fatal(e)
	}
	if e = q.complete(ctx, task, owner, map[string]any{"settlement_eligible": true}, nil, nil); e == nil {
		t.Fatal("cancelled task completed")
	}
	_ = q.fail(ctx, task, owner, "failed")
	out, e := store.GetJob(ctx, "tenant", j.JobID)
	if e != nil || out.Status != jobs.StatusCancelled || out.ResultJSON != "" {
		t.Fatal("cancel overwritten", out, e)
	}
}
func TestAtomicSnapshotFailureAndEncryptedInput(t *testing.T) {
	q, store := testQueue(t)
	ctx := context.Background()
	j := submitTask(t, q)
	var ciphertext string
	if e := q.DB.QueryRow(`SELECT payload FROM work_inputs WHERE job_id=?`, j.JobID).Scan(&ciphertext); e != nil {
		t.Fatal(e)
	}
	if !strings.HasPrefix(ciphertext, "enc1:") || strings.Contains(ciphertext, "sensitive-marker") {
		t.Fatal("input stored plaintext")
	}
	task, owner, e := q.claim(ctx)
	if e != nil {
		t.Fatal(e)
	}
	snap := &evidence.EvidenceSnapshot{SnapshotID: "snapshot", TenantID: "tenant", EvaluationID: j.JobID, Hash: "wrong", CreatedAt: time.Now()}
	if e = q.complete(ctx, task, owner, map[string]any{"delivered": true}, snap, nil); e == nil {
		t.Fatal("invalid snapshot committed")
	}
	out, e := store.GetJob(ctx, "tenant", j.JobID)
	if e != nil || out.Status != jobs.StatusRunning || out.ResultJSON != "" {
		t.Fatal("partial delivery committed", out, e)
	}
	if e = q.fail(ctx, task, owner, "internal"); e != nil {
		t.Fatal(e)
	}
	out, e = store.GetJob(ctx, "tenant", j.JobID)
	if e != nil || out.Status != jobs.StatusFailed || !strings.Contains(out.ErrorJSON, "EXECUTION_FAILED") {
		t.Fatal("failure not recorded", out, e)
	}
}
func TestWorkerSurvivesCallerDisconnect(t *testing.T) {
	q, store := testQueue(t)
	q.Handle = func(context.Context, Task) (any, *evidence.EvidenceSnapshot, error) {
		return map[string]string{"status": "done"}, nil, nil
	}
	caller, cancel := context.WithCancel(context.Background())
	j, _, e := q.Submit(caller, "tenant", "acmg_amp_2015", "simulation", "", "hash", []byte(`{}`))
	if e != nil {
		t.Fatal(e)
	}
	cancel()
	q.Start(context.Background())
	defer q.Close()
	until := time.Now().Add(3 * time.Second)
	for time.Now().Before(until) {
		out, e := store.GetJob(context.Background(), "tenant", j.JobID)
		if e != nil {
			t.Fatal(e)
		}
		if out.Status == jobs.StatusSucceeded {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("caller disconnect killed task")
}
func TestTamperedInputFailsInsteadOfBlockingQueue(t *testing.T) {
	q, store := testQueue(t)
	j := submitTask(t, q)
	if _, e := q.DB.Exec(`UPDATE work_inputs SET payload='enc1:corrupt' WHERE job_id=?`, j.JobID); e != nil {
		t.Fatal(e)
	}
	_, _, _ = q.claim(context.Background())
	out, e := store.GetJob(context.Background(), "tenant", j.JobID)
	if e != nil || out.Status != jobs.StatusFailed {
		t.Fatal("tampered input remains active", out, e)
	}
}
