package jobs

import (
	"context"
	"database/sql"
	"github.com/pzweuj/sepilens/internal/secure"
)

type EncryptedStore struct {
	*SQLiteStore
	Cipher *secure.Cipher
}

func NewEncryptedStore(db *sql.DB, c *secure.Cipher) *EncryptedStore {
	return &EncryptedStore{NewSQLiteStore(db), c}
}
func (s *EncryptedStore) CreateJob(ctx context.Context, j Job) error {
	var e error
	if j.ResultJSON != "" {
		j.ResultJSON, e = s.Cipher.Seal(j.ResultJSON, j.TenantID+":"+j.JobID+":result")
		if e != nil {
			return e
		}
	}
	if j.ErrorJSON != "" {
		j.ErrorJSON, e = s.Cipher.Seal(j.ErrorJSON, j.TenantID+":"+j.JobID+":error")
		if e != nil {
			return e
		}
	}
	return s.SQLiteStore.CreateJob(ctx, j)
}
func (s *EncryptedStore) decrypt(j Job) (Job, error) {
	var e error
	if j.ResultJSON != "" {
		j.ResultJSON, e = s.Cipher.Open(j.ResultJSON, j.TenantID+":"+j.JobID+":result")
		if e != nil {
			return j, e
		}
	}
	if j.ErrorJSON != "" {
		j.ErrorJSON, e = s.Cipher.Open(j.ErrorJSON, j.TenantID+":"+j.JobID+":error")
	}
	return j, e
}
func (s *EncryptedStore) GetJob(ctx context.Context, t, id string) (Job, error) {
	j, e := s.SQLiteStore.GetJob(ctx, t, id)
	if e != nil {
		return j, e
	}
	return s.decrypt(j)
}
func (s *EncryptedStore) FindByIdempotency(ctx context.Context, t, k string) (*Job, error) {
	j, e := s.SQLiteStore.FindByIdempotency(ctx, t, k)
	if e != nil || j == nil {
		return j, e
	}
	v, e := s.decrypt(*j)
	return &v, e
}
func (s *EncryptedStore) UpdateJobStatus(ctx context.Context, id string, status JobStatus, stage string, p int, result, problem *string) error {
	var tenant string
	if e := s.db.QueryRowContext(ctx, `SELECT tenant_id FROM jobs WHERE job_id=?`, id).Scan(&tenant); e != nil {
		return e
	}
	var r, x *string
	if result != nil {
		v, e := s.Cipher.Seal(*result, tenant+":"+id+":result")
		if e != nil {
			return e
		}
		r = &v
	}
	if problem != nil {
		v, e := s.Cipher.Seal(*problem, tenant+":"+id+":error")
		if e != nil {
			return e
		}
		x = &v
	}
	return s.SQLiteStore.UpdateJobStatus(ctx, id, status, stage, p, r, x)
}
func (s *EncryptedStore) AddReview(ctx context.Context, r ReviewRecord) error {
	v, e := s.Cipher.Seal(r.Reason, r.TenantID+":"+r.EvaluationID+":review")
	if e != nil {
		return e
	}
	r.Reason = v
	return s.SQLiteStore.AddReview(ctx, r)
}
func (s *EncryptedStore) GetReviews(ctx context.Context, t, id string) ([]ReviewRecord, error) {
	rs, e := s.SQLiteStore.GetReviews(ctx, t, id)
	if e != nil {
		return nil, e
	}
	for i := range rs {
		rs[i].Reason, e = s.Cipher.Open(rs[i].Reason, t+":"+id+":review")
		if e != nil {
			return nil, e
		}
	}
	return rs, nil
}
