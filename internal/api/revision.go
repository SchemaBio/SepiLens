package api

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/pzweuj/sepilens/internal/engine"
	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/jobs"
	"github.com/pzweuj/sepilens/internal/workflow"
	"net/http"
	"time"
)

func validClassification(v string) bool {
	return v == "Pathogenic" || v == "Likely pathogenic" || v == "VUS" || v == "Likely benign" || v == "Benign"
}
func (s *Server) reviseEvidence(w http.ResponseWriter, r *http.Request, j jobs.Job, req AddReviewRequest) {
	if s.queue == nil || j.ResultKind != "production" || len(req.Corrections) == 0 {
		WriteError(w, r, 409, ErrCodeRulesetNotAvailable, "production evaluation and evidence corrections required", false)
		return
	}
	ctx := r.Context()
	snap, e := s.evidenceStore.GetSnapshot(ctx, j.TenantID, j.JobID)
	if e != nil {
		WriteError(w, r, 409, ErrCodeBadRequest, "verified snapshot required", false)
		return
	}
	var enc string
	if s.db.QueryRowContext(ctx, `SELECT payload FROM work_inputs WHERE job_id=? AND tenant_id=?`, j.JobID, j.TenantID).Scan(&enc) != nil {
		WriteError(w, r, 409, ErrCodeBadRequest, "replay input unavailable", false)
		return
	}
	plain, e := s.queue.Cipher.Open(enc, j.TenantID+":"+j.JobID+":input")
	var in envelope
	if e != nil || json.Unmarshal([]byte(plain), &in) != nil {
		WriteError(w, r, 500, ErrCodeInternalError, "replay integrity failed", false)
		return
	}
	a, ok := s.approvals[engine.EngineID(j.EngineID)]
	if !ok || a.ContentHash != in.RulesetHash || in.DatasetHash != s.datasetHash() {
		WriteError(w, r, 409, ErrCodeRulesetNotAvailable, "pinned rule and dataset required", false)
		return
	}
	verified := map[string]bool{}
	for _, id := range req.VerifiedSources {
		verified[id] = true
	}
	for i := range snap.Sources {
		src := &snap.Sources[i]
		if verified[src.ID] {
			if !src.RecordExists || !src.IdentifierValid {
				WriteError(w, r, 422, ErrCodeBadRequest, "source record must first be resolved", false)
				return
			}
			src.ObjectMatched = true
			src.MaterialVerified = true
			src.ValidationStatus = evidence.ValidationVerified
			src.VerifiedBy = req.ReviewerID
			src.VerifiedAt = time.Now().UTC().Format(time.RFC3339Nano)
			src.VerificationReason = req.Reason
			delete(verified, src.ID)
		}
	}
	if len(verified) > 0 {
		WriteError(w, r, 422, ErrCodeBadRequest, "unknown source identity", false)
		return
	}
	req.Corrections = evidence.NewSourceVerifier().EnforceEvidenceIntegrity(req.Corrections, snap.Sources)
	for _, it := range req.Corrections {
		if it.Status != evidence.StatusMet && it.Status != evidence.StatusNotMet && it.Status != evidence.StatusNotApplicable {
			WriteError(w, r, 422, ErrCodeBadRequest, "correction requires verified source and explicit applicability", false)
			return
		}
		if it.EngineID != j.EngineID {
			WriteError(w, r, 422, ErrCodeBadRequest, "evidence engine mismatch", false)
			return
		}
	}
	var result EvaluationResultPayload
	if json.Unmarshal([]byte(j.ResultJSON), &result) != nil {
		WriteError(w, r, 500, ErrCodeInternalError, "result unavailable", false)
		return
	}
	result.RuleEngineResult = s.evaluate(in, snap.Sources, req.Corrections)
	result.Revision++
	result.ReviewStatus = "revised_pending"
	result.ReviewerDecision = nil
	snap.SnapshotID = workflow.ID("snp-")
	snap.Items = req.Corrections
	snap.CreatedAt = time.Now().UTC()
	snap.Hash, e = evidence.ComputeSnapshotHash(snap.Sources, snap.Items, snap.Provenance)
	if e == nil {
		e = s.commitRevision(ctx, j, req, result, snap)
	}
	if e != nil {
		WriteError(w, r, 500, ErrCodeInternalError, "revision commit failed", true)
		return
	}
	writeJSON(w, 201, map[string]any{"evaluation_id": j.JobID, "status": "revised_pending", "snapshot_id": snap.SnapshotID})
}
func (s *Server) commitRevision(ctx context.Context, j jobs.Job, req AddReviewRequest, result EvaluationResultPayload, snap *evidence.EvidenceSnapshot) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var version int
	if e = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0)+1 FROM audit_versions WHERE tenant_id=? AND evaluation_id=?`, j.TenantID, j.JobID).Scan(&version); e != nil {
		return e
	}
	b, e := json.Marshal(result)
	if e != nil {
		return e
	}
	enc, e := s.queue.Cipher.Seal(string(b), j.TenantID+":"+j.JobID+":result")
	if e != nil {
		return e
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	oldEnc, e := s.queue.Cipher.Seal(j.ResultJSON, j.TenantID+":"+j.JobID+":result")
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO audit_versions VALUES(?,?,?,?,?)`, j.TenantID, j.JobID, version, oldEnc, now); e != nil {
		return e
	}
	res, e := tx.ExecContext(ctx, `UPDATE jobs SET result_json=?,updated_at=? WHERE tenant_id=? AND job_id=? AND status='succeeded' AND updated_at=?`, enc, now, j.TenantID, j.JobID, j.UpdatedAt.Format(time.RFC3339Nano))
	if e != nil {
		return e
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return errors.New("concurrent revision or cancellation")
	}
	es := s.queue.Evidence
	if e = es.SaveTx(ctx, tx, *snap); e != nil {
		return e
	}
	reason, e := s.queue.Cipher.Seal(req.Reason, j.TenantID+":"+j.JobID+":review")
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO reviews(evaluation_id,tenant_id,reviewer_id,decision,reason,human_override,reviewed_classification,created_at,result_version) VALUES(?,?,?,?,?,0,'',?,?)`, j.JobID, j.TenantID, req.ReviewerID, req.Decision, reason, now, result.Revision)
	if e != nil {
		return e
	}
	return tx.Commit()
}
