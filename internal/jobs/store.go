package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type JobStatus string

const (
	StatusQueued    JobStatus = "queued"
	StatusRunning   JobStatus = "running"
	StatusSucceeded JobStatus = "succeeded"
	StatusFailed    JobStatus = "failed"
	StatusCancelled JobStatus = "cancelled"
)

var (
	ErrJobNotFound        = errors.New("job not found")
	ErrJobAlreadyFinished = errors.New("job is already in a terminal state")
)

type Job struct {
	JobID          string    `json:"job_id"`
	TenantID       string    `json:"tenant_id"`
	EngineID       string    `json:"engine_id"`
	Status         JobStatus `json:"status"`
	ResultKind     string    `json:"result_kind"`
	RequestHash    string    `json:"request_hash"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
	Stage          string    `json:"stage"`
	Progress       int       `json:"progress"`
	ResultJSON     string    `json:"result_json,omitempty"`
	ErrorJSON      string    `json:"error_json,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type JobEvent struct {
	ID          int64     `json:"id"`
	JobID       string    `json:"job_id"`
	Seq         int       `json:"seq"`
	Stage       string    `json:"stage"`
	PayloadJSON string    `json:"payload_json"`
	CreatedAt   time.Time `json:"created_at"`
}

type ReviewRecord struct {
	ID                     int64     `json:"id"`
	EvaluationID           string    `json:"evaluation_id"`
	TenantID               string    `json:"tenant_id"`
	ReviewerID             string    `json:"reviewer_id"`
	Decision               string    `json:"decision"`
	Reason                 string    `json:"reason"`
	HumanOverride          bool      `json:"human_override"`
	ReviewedClassification string    `json:"reviewed_classification"`
	CreatedAt              time.Time `json:"created_at"`
}

type Store interface {
	CreateJob(ctx context.Context, job Job) error
	GetJob(ctx context.Context, tenantID, jobID string) (Job, error)
	FindByIdempotency(ctx context.Context, tenantID, idempotencyKey string) (*Job, error)
	UpdateJobStatus(ctx context.Context, jobID string, status JobStatus, stage string, progress int, resultJSON, errorJSON *string) error
	AppendEvent(ctx context.Context, jobID string, stage string, payloadJSON string) (JobEvent, error)
	GetEvents(ctx context.Context, jobID string, afterSeq int) ([]JobEvent, error)
	CancelJob(ctx context.Context, tenantID, jobID string) error
	AddReview(ctx context.Context, review ReviewRecord) error
	GetReviews(ctx context.Context, tenantID, evaluationID string) ([]ReviewRecord, error)
}

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

func (s *SQLiteStore) CreateJob(ctx context.Context, j Job) error {
	now := time.Now().UTC()
	if j.CreatedAt.IsZero() {
		j.CreatedAt = now
	}
	if j.UpdatedAt.IsZero() {
		j.UpdatedAt = now
	}

	query := `INSERT INTO jobs (
		job_id, tenant_id, engine_id, status, result_kind, request_hash,
		idempotency_key, stage, progress, result_json, error_json, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := s.db.ExecContext(ctx, query,
		j.JobID, j.TenantID, j.EngineID, string(j.Status), j.ResultKind, j.RequestHash,
		nullString(j.IdempotencyKey), j.Stage, j.Progress, nullString(j.ResultJSON),
		nullString(j.ErrorJSON), j.CreatedAt.Format(time.RFC3339Nano), j.UpdatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("insert job: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetJob(ctx context.Context, tenantID, jobID string) (Job, error) {
	query := `SELECT job_id, tenant_id, engine_id, status, result_kind, request_hash,
		COALESCE(idempotency_key, ''), stage, progress, COALESCE(result_json, ''),
		COALESCE(error_json, ''), created_at, updated_at
		FROM jobs WHERE tenant_id = ? AND job_id = ?`

	var j Job
	var statusStr, createdStr, updatedStr string
	err := s.db.QueryRowContext(ctx, query, tenantID, jobID).Scan(
		&j.JobID, &j.TenantID, &j.EngineID, &statusStr, &j.ResultKind, &j.RequestHash,
		&j.IdempotencyKey, &j.Stage, &j.Progress, &j.ResultJSON, &j.ErrorJSON,
		&createdStr, &updatedStr,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrJobNotFound
	}
	if err != nil {
		return Job{}, fmt.Errorf("query job: %w", err)
	}

	j.Status = JobStatus(statusStr)
	j.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
	j.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedStr)
	return j, nil
}

func (s *SQLiteStore) FindByIdempotency(ctx context.Context, tenantID, idempotencyKey string) (*Job, error) {
	if idempotencyKey == "" {
		return nil, nil
	}
	query := `SELECT job_id, tenant_id, engine_id, status, result_kind, request_hash,
		COALESCE(idempotency_key, ''), stage, progress, COALESCE(result_json, ''),
		COALESCE(error_json, ''), created_at, updated_at
		FROM jobs WHERE tenant_id = ? AND idempotency_key = ?`

	var j Job
	var statusStr, createdStr, updatedStr string
	err := s.db.QueryRowContext(ctx, query, tenantID, idempotencyKey).Scan(
		&j.JobID, &j.TenantID, &j.EngineID, &statusStr, &j.ResultKind, &j.RequestHash,
		&j.IdempotencyKey, &j.Stage, &j.Progress, &j.ResultJSON, &j.ErrorJSON,
		&createdStr, &updatedStr,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find idempotency: %w", err)
	}

	j.Status = JobStatus(statusStr)
	j.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
	j.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedStr)
	return &j, nil
}

func (s *SQLiteStore) UpdateJobStatus(ctx context.Context, jobID string, status JobStatus, stage string, progress int, resultJSON, errorJSON *string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	query := `UPDATE jobs SET status = ?, stage = ?, progress = ?,
		result_json = COALESCE(?, result_json),
		error_json = COALESCE(?, error_json),
		updated_at = ?
		WHERE job_id = ?`

	res, err := s.db.ExecContext(ctx, query, string(status), stage, progress, resultJSON, errorJSON, now, jobID)
	if err != nil {
		return fmt.Errorf("update job status: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrJobNotFound
	}
	return nil
}

func (s *SQLiteStore) AppendEvent(ctx context.Context, jobID string, stage string, payloadJSON string) (JobEvent, error) {
	now := time.Now().UTC()
	var nextSeq int
	seqQuery := `SELECT COALESCE(MAX(seq), 0) + 1 FROM job_events WHERE job_id = ?`
	if err := s.db.QueryRowContext(ctx, seqQuery, jobID).Scan(&nextSeq); err != nil {
		return JobEvent{}, fmt.Errorf("compute next event seq: %w", err)
	}

	insQuery := `INSERT INTO job_events (job_id, seq, stage, payload_json, created_at)
		VALUES (?, ?, ?, ?, ?)`
	res, err := s.db.ExecContext(ctx, insQuery, jobID, nextSeq, stage, payloadJSON, now.Format(time.RFC3339Nano))
	if err != nil {
		return JobEvent{}, fmt.Errorf("insert event: %w", err)
	}
	id, _ := res.LastInsertId()

	return JobEvent{
		ID:          id,
		JobID:       jobID,
		Seq:         nextSeq,
		Stage:       stage,
		PayloadJSON: payloadJSON,
		CreatedAt:   now,
	}, nil
}

func (s *SQLiteStore) GetEvents(ctx context.Context, jobID string, afterSeq int) ([]JobEvent, error) {
	query := `SELECT id, job_id, seq, stage, payload_json, created_at
		FROM job_events WHERE job_id = ? AND seq > ? ORDER BY seq ASC`

	rows, err := s.db.QueryContext(ctx, query, jobID, afterSeq)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []JobEvent
	for rows.Next() {
		var e JobEvent
		var createdStr string
		if err := rows.Scan(&e.ID, &e.JobID, &e.Seq, &e.Stage, &e.PayloadJSON, &createdStr); err != nil {
			return nil, err
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *SQLiteStore) CancelJob(ctx context.Context, tenantID, jobID string) error {
	j, err := s.GetJob(ctx, tenantID, jobID)
	if err != nil {
		return err
	}
	if j.Status == StatusSucceeded || j.Status == StatusFailed || j.Status == StatusCancelled {
		return ErrJobAlreadyFinished
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	query := `UPDATE jobs SET status = ?, stage = 'cancelled', updated_at = ? WHERE tenant_id = ? AND job_id = ?`
	_, err = s.db.ExecContext(ctx, query, string(StatusCancelled), now, tenantID, jobID)
	return err
}

func (s *SQLiteStore) AddReview(ctx context.Context, r ReviewRecord) error {
	now := time.Now().UTC()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	overrideInt := 0
	if r.HumanOverride {
		overrideInt = 1
	}

	query := `INSERT INTO reviews (
		evaluation_id, tenant_id, reviewer_id, decision, reason,
		human_override, reviewed_classification, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := s.db.ExecContext(ctx, query,
		r.EvaluationID, r.TenantID, r.ReviewerID, r.Decision, r.Reason,
		overrideInt, nullString(r.ReviewedClassification), r.CreatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("insert review: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetReviews(ctx context.Context, tenantID, evaluationID string) ([]ReviewRecord, error) {
	query := `SELECT id, evaluation_id, tenant_id, reviewer_id, decision, reason,
		human_override, COALESCE(reviewed_classification, ''), created_at
		FROM reviews WHERE tenant_id = ? AND evaluation_id = ? ORDER BY id ASC`

	rows, err := s.db.QueryContext(ctx, query, tenantID, evaluationID)
	if err != nil {
		return nil, fmt.Errorf("query reviews: %w", err)
	}
	defer rows.Close()

	var reviews []ReviewRecord
	for rows.Next() {
		var r ReviewRecord
		var overrideInt int
		var createdStr string
		if err := rows.Scan(&r.ID, &r.EvaluationID, &r.TenantID, &r.ReviewerID, &r.Decision, &r.Reason, &overrideInt, &r.ReviewedClassification, &createdStr); err != nil {
			return nil, err
		}
		r.HumanOverride = overrideInt == 1
		r.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		reviews = append(reviews, r)
	}
	return reviews, rows.Err()
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
