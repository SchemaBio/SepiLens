package workflow

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/jobs"
	"github.com/pzweuj/sepilens/internal/secure"
	"sync"
	"time"
)

var ErrConflict = errors.New("idempotency conflict")

type Handler func(context.Context, Task) (any, *evidence.EvidenceSnapshot, error)

// PartialFailure allows an audited local baseline to survive an AI failure.
// Codes are application-owned constants; provider bodies must never reach here.
type PartialFailure struct{ Code string }

func (p *PartialFailure) Error() string { return p.Code }

type Task struct {
	JobID, TenantID, EngineID, Kind string
	Payload                         []byte
	External                        func() error
}
type Queue struct {
	DB       *sql.DB
	Cipher   *secure.Cipher
	Evidence *evidence.SQLiteStore
	Handle   Handler
	wg       sync.WaitGroup
	cancel   context.CancelFunc
	Workers  int
}

func ID(prefix string) string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic("random source unavailable")
	}
	return prefix + hex.EncodeToString(b)
}
func (q *Queue) Submit(ctx context.Context, t, engine, kind, key, hash string, payload []byte) (jobs.Job, bool, error) {
	if len(key) > 256 {
		return jobs.Job{}, false, errors.New("idempotency key too long")
	}
	if key != "" {
		digest := sha256.Sum256([]byte(key))
		key = hex.EncodeToString(digest[:])
	}
	tx, e := q.DB.BeginTx(ctx, nil)
	if e != nil {
		return jobs.Job{}, false, e
	}
	defer tx.Rollback()
	if key != "" {
		var j jobs.Job
		e = tx.QueryRowContext(ctx, `SELECT job_id,request_hash,status,result_kind FROM jobs WHERE tenant_id=? AND idempotency_key=?`, t, key).Scan(&j.JobID, &j.RequestHash, &j.Status, &j.ResultKind)
		if e == nil {
			if j.RequestHash != hash {
				return j, true, ErrConflict
			}
			return j, true, nil
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return j, false, e
		}
	}
	now := time.Now().UTC()
	j := jobs.Job{JobID: ID("job-"), TenantID: t, EngineID: engine, ResultKind: kind, Status: jobs.StatusQueued, Stage: "queued", RequestHash: hash, CreatedAt: now, UpdatedAt: now}
	enc, e := q.Cipher.Seal(string(payload), t+":"+j.JobID+":input")
	if e != nil {
		return j, false, e
	}
	var idem any
	if key != "" {
		idem = key
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO jobs(job_id,tenant_id,engine_id,status,result_kind,request_hash,idempotency_key,stage,created_at,updated_at) VALUES (?,?,?,'queued',?,?,?,'queued',?,?)`, j.JobID, t, engine, kind, hash, idem, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if e != nil {
		return j, false, e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO work_inputs(job_id,tenant_id,payload) VALUES(?,?,?)`, j.JobID, t, enc)
	if e != nil {
		return j, false, e
	}
	if e = event(ctx, tx, j.JobID, "queued"); e != nil {
		return j, false, e
	}
	return j, false, tx.Commit()
}
func event(ctx context.Context, tx *sql.Tx, id, stage string) error {
	_, e := tx.ExecContext(ctx, `INSERT INTO job_events(job_id,seq,stage,payload_json,created_at) SELECT ?,COALESCE(MAX(seq),0)+1,?,?,? FROM job_events WHERE job_id=?`, id, stage, `{"stage":"`+stage+`"}`, time.Now().UTC().Format(time.RFC3339Nano), id)
	return e
}
func (q *Queue) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	q.cancel = cancel
	n := q.Workers
	if n <= 0 {
		n = 2
	}
	for i := 0; i < n; i++ {
		q.wg.Add(1)
		go func() {
			defer q.wg.Done()
			tick := time.NewTicker(250 * time.Millisecond)
			defer tick.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
					q.runOne(ctx)
				}
			}
		}()
	}
}
func (q *Queue) Close() {
	if q.cancel != nil {
		q.cancel()
	}
	q.wg.Wait()
}
func (q *Queue) claim(ctx context.Context) (Task, string, error) {
	tx, e := q.DB.BeginTx(ctx, nil)
	if e != nil {
		return Task{}, "", e
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// Uncertain external calls are never retried after lease loss.
	_, e = tx.ExecContext(ctx, `INSERT INTO job_events(job_id,seq,stage,payload_json,created_at) SELECT j.job_id,COALESCE((SELECT MAX(seq) FROM job_events WHERE job_id=j.job_id),0)+1,'interrupted','{"stage":"interrupted","regeneration_required":true}',? FROM jobs j JOIN work_inputs w ON w.job_id=j.job_id WHERE j.status='running' AND w.external_started=1 AND w.lease_until<?`, now, now)
	if e != nil {
		return Task{}, "", e
	}
	_, e = tx.ExecContext(ctx, `UPDATE jobs SET status='failed',stage='interrupted',updated_at=? WHERE status='running' AND job_id IN (SELECT job_id FROM work_inputs WHERE external_started=1 AND lease_until<?)`, now, now)
	if e != nil {
		return Task{}, "", e
	}
	var task Task
	var enc string
	e = tx.QueryRowContext(ctx, `SELECT j.job_id,j.tenant_id,j.engine_id,j.result_kind,w.payload FROM jobs j JOIN work_inputs w ON w.job_id=j.job_id WHERE j.status='queued' OR (j.status='running' AND w.external_started=0 AND w.lease_until<?) ORDER BY j.created_at LIMIT 1`, now).Scan(&task.JobID, &task.TenantID, &task.EngineID, &task.Kind, &enc)
	if e != nil {
		if errors.Is(e, sql.ErrNoRows) {
			if err := tx.Commit(); err != nil {
				return task, "", err
			}
		}
		return task, "", e
	}
	p, e := q.Cipher.Open(enc, task.TenantID+":"+task.JobID+":input")
	if e != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE jobs SET status='failed',stage='input_integrity_failed',updated_at=? WHERE job_id=?`, now, task.JobID); err != nil {
			return task, "", err
		}
		if err := event(ctx, tx, task.JobID, "input_integrity_failed"); err != nil {
			return task, "", err
		}
		if err := tx.Commit(); err != nil {
			return task, "", err
		}
		return task, "", e
	}
	task.Payload = []byte(p)
	owner := ID("lease-")
	_, e = tx.ExecContext(ctx, `UPDATE work_inputs SET owner=?,lease_until=? WHERE job_id=?`, owner, time.Now().UTC().Add(30*time.Second).Format(time.RFC3339Nano), task.JobID)
	if e != nil {
		return task, "", e
	}
	_, e = tx.ExecContext(ctx, `UPDATE jobs SET status='running',stage='running',updated_at=? WHERE job_id=?`, now, task.JobID)
	if e != nil {
		return task, "", e
	}
	if e = event(ctx, tx, task.JobID, "running"); e != nil {
		return task, "", e
	}
	return task, owner, tx.Commit()
}
func (q *Queue) runOne(parent context.Context) {
	task, owner, e := q.claim(parent)
	if e != nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 300*time.Second)
	defer cancel()
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-tick.C:
				var status string
				if q.DB.QueryRowContext(ctx, `SELECT status FROM jobs WHERE job_id=?`, task.JobID).Scan(&status) != nil || status != "running" {
					cancel()
					return
				}
				if _, err := q.DB.ExecContext(ctx, `UPDATE work_inputs SET lease_until=? WHERE job_id=? AND owner=?`, time.Now().UTC().Add(30*time.Second).Format(time.RFC3339Nano), task.JobID, owner); err != nil {
					cancel()
					return
				}
			}
		}
	}()
	task.External = func() error {
		res, err := q.DB.ExecContext(ctx, `UPDATE work_inputs SET external_started=1 WHERE job_id=? AND owner=? AND EXISTS(SELECT 1 FROM jobs WHERE job_id=? AND status='running')`, task.JobID, owner, task.JobID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return errors.New("task lease lost")
		}
		return nil
	}
	var result any
	var snapshot *evidence.EvidenceSnapshot
	func() {
		defer func() {
			if recover() != nil {
				e = errors.New("task execution failed")
			}
		}()
		if q.Handle == nil {
			e = errors.New("task handler unavailable")
			return
		}
		result, snapshot, e = q.Handle(ctx, task)
	}()
	if parent.Err() != nil {
		return
	}
	commitCtx, done := context.WithTimeout(context.Background(), 10*time.Second)
	defer done()
	if err := q.complete(commitCtx, task, owner, result, snapshot, e); err != nil {
		_ = q.fail(commitCtx, task, owner, "storage commit failed")
	}
}
func (q *Queue) fail(ctx context.Context, t Task, owner, msg string) error {
	tx, e := q.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	// Callers pass only internal error codes; never persist provider response bodies.
	enc, e := q.Cipher.Seal(`{"code":"EXECUTION_FAILED","regeneration_required":true}`, t.TenantID+":"+t.JobID+":error")
	if e != nil {
		return e
	}
	res, e := tx.ExecContext(ctx, `UPDATE jobs SET status='failed',stage='failed',error_json=?,updated_at=? WHERE job_id=? AND status='running' AND EXISTS(SELECT 1 FROM work_inputs WHERE job_id=? AND owner=?)`, enc, time.Now().UTC().Format(time.RFC3339Nano), t.JobID, t.JobID, owner)
	if e != nil {
		return e
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return nil
	}
	if e = event(ctx, tx, t.JobID, "failed"); e != nil {
		return e
	}
	return tx.Commit()
}
func (q *Queue) complete(ctx context.Context, t Task, owner string, result any, snapshot *evidence.EvidenceSnapshot, problem error) error {
	var partial *PartialFailure
	if problem != nil && (!errors.As(problem, &partial) || result == nil || snapshot == nil) {
		return q.fail(ctx, t, owner, "execution failed")
	}
	b, e := json.Marshal(result)
	if e != nil {
		return e
	}
	enc, e := q.Cipher.Seal(string(b), t.TenantID+":"+t.JobID+":result")
	if e != nil {
		return e
	}
	tx, e := q.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	status, stage := "succeeded", "completed"
	var errorJSON any
	if partial != nil {
		status, stage = "failed", "failed"
		code := partial.Code
		switch code {
		case "AI_OUTPUT_INVALID", "AI_PROVIDER_UNAVAILABLE", "AI_TOOL_BUDGET_EXCEEDED":
		default:
			code = "EXECUTION_FAILED"
		}
		raw, _ := json.Marshal(map[string]any{"code": code, "regeneration_required": true})
		errorJSON, e = q.Cipher.Seal(string(raw), t.TenantID+":"+t.JobID+":error")
		if e != nil {
			return e
		}
	}
	res, e := tx.ExecContext(ctx, `UPDATE jobs SET status=?,stage=?,progress=100,result_json=?,error_json=?,updated_at=? WHERE job_id=? AND tenant_id=? AND status='running' AND EXISTS(SELECT 1 FROM work_inputs WHERE job_id=? AND owner=?)`, status, stage, enc, errorJSON, time.Now().UTC().Format(time.RFC3339Nano), t.JobID, t.TenantID, t.JobID, owner)
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return errors.New("task cancelled or lease lost")
	}
	if snapshot != nil {
		if q.Evidence == nil {
			return errors.New("evidence store required")
		}
		if e = q.Evidence.SaveTx(ctx, tx, *snapshot); e != nil {
			return e
		}
	}
	if e = event(ctx, tx, t.JobID, stage); e != nil {
		return e
	}
	return tx.Commit()
}
func (q *Queue) Purge(ctx context.Context, tenant, id string) error {
	tx, e := q.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var status string
	if e = tx.QueryRowContext(ctx, `SELECT status FROM jobs WHERE tenant_id=? AND job_id=?`, tenant, id).Scan(&status); e != nil {
		return jobs.ErrJobNotFound
	}
	if status == "running" || status == "queued" {
		return errors.New("cancel active job before deletion")
	}
	for _, table := range []string{"job_events", "work_inputs"} {
		if _, e = tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE job_id=?", table), id); e != nil {
			return e
		}
	}
	for _, table := range []string{"reviews", "evidence_snapshots", "audit_versions"} {
		if _, e = tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE tenant_id=? AND evaluation_id=?", table), tenant, id); e != nil {
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM jobs WHERE tenant_id=? AND job_id=?`, tenant, id); e != nil {
		return e
	}
	return tx.Commit()
}
func (q *Queue) Expire(ctx context.Context, days int) error {
	if days <= 0 {
		days = 90
	}
	rows, e := q.DB.QueryContext(ctx, `SELECT tenant_id,job_id FROM jobs WHERE status NOT IN ('queued','running') AND created_at<?`, time.Now().UTC().AddDate(0, 0, -days).Format(time.RFC3339Nano))
	if e != nil {
		return e
	}
	var ids [][2]string
	for rows.Next() {
		var p [2]string
		if e = rows.Scan(&p[0], &p[1]); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, p)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, p := range ids {
		if e = q.Purge(ctx, p[0], p[1]); e != nil {
			return e
		}
	}
	return nil
}
