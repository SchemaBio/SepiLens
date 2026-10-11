package api

import (
	"net/http"
)

func (s *Server) handleDeleteTenantData(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, "admin") {
		return
	}
	if s.queue == nil {
		WriteError(w, r, 503, ErrCodeInternalError, "store unavailable", false)
		return
	}
	tenant := GetTenantID(r.Context())
	tx, e := s.db.BeginTx(r.Context(), nil)
	if e != nil {
		WriteError(w, r, 500, ErrCodeInternalError, "deletion failed", true)
		return
	}
	defer tx.Rollback()
	statements := []string{
		`DELETE FROM job_events WHERE job_id IN (SELECT job_id FROM jobs WHERE tenant_id=?)`,
		`DELETE FROM work_inputs WHERE tenant_id=?`,
		`DELETE FROM reviews WHERE tenant_id=?`,
		`DELETE FROM evidence_snapshots WHERE tenant_id=?`,
		`DELETE FROM audit_versions WHERE tenant_id=?`,
		`DELETE FROM jobs WHERE tenant_id=?`,
		`INSERT OR IGNORE INTO profile_deleted SELECT DISTINCT tenant_id,profile_id FROM provider_profiles WHERE tenant_id=?`,
		`DELETE FROM provider_profiles WHERE tenant_id=?`,
	}
	for _, sql := range statements {
		if _, e = tx.ExecContext(r.Context(), sql, tenant); e != nil {
			WriteError(w, r, 500, ErrCodeInternalError, "deletion failed", true)
			return
		}
	}
	if e = tx.Commit(); e != nil {
		WriteError(w, r, 500, ErrCodeInternalError, "deletion failed", true)
		return
	}
	w.WriteHeader(204)
}
