package api

import (
	"context"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/pzweuj/sepilens/internal/engine"
	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/hpo"
	"github.com/pzweuj/sepilens/internal/jobs"
	"github.com/pzweuj/sepilens/internal/profiles"
	"github.com/pzweuj/sepilens/internal/secure"
	"github.com/pzweuj/sepilens/internal/workflow"
	"net/http"
	"time"
)

func (s *Server) Start(ctx context.Context) error {
	if s.queue != nil {
		return errors.New("server already started")
	}
	if s.cfg.HPOKnowledgeDir != "" {
		index, err := hpo.LoadDefaultIndex(s.cfg.HPOKnowledgeDir)
		if err != nil {
			return errors.New("HPO terminology could not be loaded")
		}
		s.hpoExtractor = hpo.NewExtractor(index, nil)
	}
	if s.db == nil {
		return errors.New("database required")
	}
	c, e := secure.New(s.cfg.MasterKey)
	if e != nil {
		return e
	}
	if e = secure.Initialize(ctx, s.db, c); e != nil {
		return e
	}
	s.store = jobs.NewEncryptedStore(s.db, c)
	es := evidence.NewEncryptedStore(s.db, c)
	s.evidenceStore = es
	s.profiles = &profiles.Store{DB: s.db, Cipher: c, AllowPrivate: s.cfg.AllowPrivateEndpoints}
	ctx, s.cancel = context.WithCancel(ctx)
	s.maintenanceDone = make(chan struct{})
	s.queue = &workflow.Queue{DB: s.db, Cipher: c, Evidence: es, Handle: s.executeV2}
	s.queue.Start(ctx)
	go func() {
		defer close(s.maintenanceDone)
		tick := time.NewTicker(time.Hour)
		defer tick.Stop()
		_ = s.queue.Expire(ctx, s.cfg.RetentionDays)
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if e := s.queue.Expire(ctx, s.cfg.RetentionDays); e != nil {
					s.logger.Error("retention cleanup failed")
				}
			}
		}
	}()
	return nil
}
func (s *Server) Close() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.queue != nil {
		s.queue.Close()
	}
	if s.maintenanceDone != nil {
		<-s.maintenanceDone
	}
}

// SetApprovalsForTest injects a synthetic approval in isolated package tests.
// Production approvals must arrive through LoadRuleApproval with a signature.
func (s *Server) SetApprovalsForTest(a map[engine.EngineID]engine.Approval) {
	s.approvals = a
}
func (s *Server) handleDeleteJob(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, "admin") {
		return
	}
	if s.queue == nil {
		WriteError(w, r, 503, ErrCodeInternalError, "worker unavailable", false)
		return
	}
	e := s.queue.Purge(r.Context(), GetTenantID(r.Context()), chi.URLParam(r, "job_id"))
	if e != nil {
		status := 409
		if errors.Is(e, jobs.ErrJobNotFound) {
			status = 404
		}
		WriteError(w, r, status, ErrCodeBadRequest, "job cannot be deleted", false)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) handleExportJob(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, "reviewer", "admin") {
		return
	}
	id := chi.URLParam(r, "job_id")
	tenant := GetTenantID(r.Context())
	j, e := s.store.GetJob(r.Context(), tenant, id)
	if e != nil {
		WriteError(w, r, 404, ErrCodeJobNotFound, "job not found", false)
		return
	}
	reviews, e := s.store.GetReviews(r.Context(), tenant, id)
	if e != nil {
		WriteError(w, r, 500, ErrCodeInternalError, "audit unavailable", false)
		return
	}
	var snapshot *evidence.EvidenceSnapshot
	if s.evidenceStore != nil {
		snapshot, e = s.evidenceStore.GetSnapshot(r.Context(), tenant, id)
		if e != nil && !errors.Is(e, evidence.ErrSnapshotNotFound) {
			WriteError(w, r, 500, ErrCodeInternalError, "audit integrity failed", false)
			return
		}
	}
	history, snapshots, e := s.auditHistory(r.Context(), tenant, id)
	if e != nil {
		WriteError(w, r, 500, ErrCodeInternalError, "audit integrity failed", false)
		return
	}
	writeJSON(w, 200, map[string]any{"job": j, "reviews": reviews, "snapshot": snapshot, "result_history": history, "evidence_history": snapshots})
}
