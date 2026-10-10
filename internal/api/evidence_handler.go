package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/pzweuj/sepilens/internal/evidence"
)

func (s *Server) handleGetEvidence(w http.ResponseWriter, r *http.Request) {
	if s.evidenceStore == nil {
		WriteError(w, r, http.StatusNotFound, ErrCodeJobNotFound, "evidence store not initialized", false)
		return
	}

	ctx := r.Context()
	tenantID := GetTenantID(ctx)
	evalID := chi.URLParam(r, "id")

	snapshot, err := s.evidenceStore.GetSnapshot(ctx, tenantID, evalID)
	if errors.Is(err, evidence.ErrSnapshotNotFound) {
		WriteError(w, r, http.StatusNotFound, ErrCodeJobNotFound, "evidence snapshot not found", false)
		return
	}
	if err != nil {
		WriteError(w, r, http.StatusInternalServerError, ErrCodeInternalError, "failed to query evidence snapshot", true)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(snapshot)
}
