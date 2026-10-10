package jobs_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/pzweuj/sepilens/internal/jobs"
	"github.com/pzweuj/sepilens/internal/storage/sqlite"
)

func setupTestStore(t *testing.T) (*jobs.SQLiteStore, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}

	// Verify idempotency of migrations by opening again
	db2, err2 := sqlite.Open(dbPath)
	if err2 != nil {
		t.Fatalf("failed to re-open sqlite db: %v", err2)
	}
	_ = db2.Close()

	store := jobs.NewSQLiteStore(db)
	return store, func() {
		_ = db.Close()
	}
}

func TestStore_JobLifecycleAndTenantIsolation(t *testing.T) {
	ctx := context.Background()
	store, teardown := setupTestStore(t)
	defer teardown()

	job1 := jobs.Job{
		JobID:          "job-100",
		TenantID:       "tenant-A",
		EngineID:       "acmg_amp_2015",
		Status:         jobs.StatusQueued,
		ResultKind:     "simulation",
		RequestHash:    "hash-100",
		IdempotencyKey: "idem-key-1",
		Stage:          "queued",
		Progress:       0,
	}

	if err := store.CreateJob(ctx, job1); err != nil {
		t.Fatalf("failed to create job: %v", err)
	}

	// Lookup by creator tenant
	fetched, err := store.GetJob(ctx, "tenant-A", "job-100")
	if err != nil {
		t.Fatalf("failed to fetch job: %v", err)
	}
	if fetched.JobID != "job-100" || fetched.Status != jobs.StatusQueued {
		t.Fatalf("unexpected fetched job: %+v", fetched)
	}

	// Tenant B cannot access tenant A's job
	_, errTenantB := store.GetJob(ctx, "tenant-B", "job-100")
	if !errors.Is(errTenantB, jobs.ErrJobNotFound) {
		t.Fatalf("expected ErrJobNotFound for other tenant, got %v", errTenantB)
	}

	// Idempotency lookup
	idemFound, err := store.FindByIdempotency(ctx, "tenant-A", "idem-key-1")
	if err != nil || idemFound == nil {
		t.Fatalf("expected to find job by idempotency key: %v", err)
	}
	if idemFound.JobID != "job-100" {
		t.Fatalf("mismatched job id on idempotency lookup: %s", idemFound.JobID)
	}

	// Events append & retrieve
	ev1, err := store.AppendEvent(ctx, "job-100", "running", `{"info":"started"}`)
	if err != nil || ev1.Seq != 1 {
		t.Fatalf("failed to append event 1: %v", err)
	}
	ev2, err := store.AppendEvent(ctx, "job-100", "running", `{"step":"scoring"}`)
	if err != nil || ev2.Seq != 2 {
		t.Fatalf("failed to append event 2: %v", err)
	}

	events, err := store.GetEvents(ctx, "job-100", 0)
	if err != nil || len(events) != 2 {
		t.Fatalf("expected 2 events, got %d (err=%v)", len(events), err)
	}

	// Status update to succeeded
	resJSON := `{"classification":""}`
	if err := store.UpdateJobStatus(ctx, "job-100", jobs.StatusSucceeded, "completed", 100, &resJSON, nil); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	updated, err := store.GetJob(ctx, "tenant-A", "job-100")
	if err != nil || updated.Status != jobs.StatusSucceeded || updated.Progress != 100 {
		t.Fatalf("job status update verification failed: %+v", updated)
	}

	// Cancellation of finished job fails
	errCancel := store.CancelJob(ctx, "tenant-A", "job-100")
	if !errors.Is(errCancel, jobs.ErrJobAlreadyFinished) {
		t.Fatalf("expected ErrJobAlreadyFinished on cancel, got %v", errCancel)
	}

	// Cancel a queued job
	job2 := jobs.Job{
		JobID:       "job-200",
		TenantID:    "tenant-A",
		EngineID:    "cnv_loss_2020",
		Status:      jobs.StatusQueued,
		ResultKind:  "simulation",
		RequestHash: "hash-200",
		CreatedAt:   time.Now().UTC(),
	}
	if err := store.CreateJob(ctx, job2); err != nil {
		t.Fatalf("failed to create job 2: %v", err)
	}
	if err := store.CancelJob(ctx, "tenant-A", "job-200"); err != nil {
		t.Fatalf("failed to cancel job 2: %v", err)
	}
	cancelled, err := store.GetJob(ctx, "tenant-A", "job-200")
	if err != nil || cancelled.Status != jobs.StatusCancelled {
		t.Fatalf("job 2 was not cancelled: %+v", cancelled)
	}
}

func TestStore_Reviews(t *testing.T) {
	ctx := context.Background()
	store, teardown := setupTestStore(t)
	defer teardown()

	rev := jobs.ReviewRecord{
		EvaluationID:           "eval-1",
		TenantID:               "tenant-A",
		ReviewerID:             "doctor-jones",
		Decision:               "override",
		Reason:                 "Expert manual override based on functional assay",
		HumanOverride:          true,
		ReviewedClassification: "Pathogenic",
	}

	if err := store.AddReview(ctx, rev); err != nil {
		t.Fatalf("failed to add review: %v", err)
	}

	records, err := store.GetReviews(ctx, "tenant-A", "eval-1")
	if err != nil || len(records) != 1 {
		t.Fatalf("failed to get reviews: %v (len=%d)", err, len(records))
	}

	r0 := records[0]
	if !r0.HumanOverride || r0.ReviewedClassification != "Pathogenic" || r0.ReviewerID != "doctor-jones" {
		t.Fatalf("mismatched review record: %+v", r0)
	}

	// Tenant B has no access to reviews of tenant A
	recordsB, err := store.GetReviews(ctx, "tenant-B", "eval-1")
	if err != nil || len(recordsB) != 0 {
		t.Fatalf("tenant B should see 0 reviews, got %d", len(recordsB))
	}
}
