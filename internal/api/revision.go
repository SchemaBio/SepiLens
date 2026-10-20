package api

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/pzweuj/sepilens/internal/assessment"
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
		WriteError(w, r, 409, ErrCodeRulesetNotAvailable, "production evidence corrections required", false)
		return
	}
	ctx := r.Context()
	snap, err := s.evidenceStore.GetSnapshot(ctx, j.TenantID, j.JobID)
	if err != nil {
		WriteError(w, r, 409, ErrCodeBadRequest, "snapshot required", false)
		return
	}
	in, err := s.savedInput(ctx, j)
	a, ok := s.approvals[engine.EngineID(j.EngineID)]
	if err != nil || !ok || a.ContentHash != in.RulesetHash || in.PolicyHash != s.policyHash() {
		WriteError(w, r, 409, ErrCodeRulesetNotAvailable, "pinned rule and source policy required", false)
		return
	}
	var result EvaluationResultPayload
	if json.Unmarshal([]byte(j.ResultJSON), &result) != nil {
		WriteError(w, r, 500, ErrCodeInternalError, "result integrity failed", false)
		return
	}
	b, _ := json.Marshal(result.RuleEngineResult)
	var effective assessment.Result
	if json.Unmarshal(b, &effective) != nil {
		WriteError(w, r, 409, ErrCodeBadRequest, "effective criteria required", false)
		return
	}
	// Source resolution and policy approval cannot be manufactured during review.
	for _, id := range req.VerifiedSources {
		found := false
		for _, src := range snap.Sources {
			found = found || (src.ID == id && src.RecordExists && src.IdentifierValid && src.ObjectMatched)
		}
		if !found {
			WriteError(w, r, 422, ErrCodeBadRequest, "unresolved source", false)
			return
		}
	}
	for i := range snap.Sources {
		src := &snap.Sources[i]
		for _, id := range req.VerifiedSources {
			if src.ID == id {
				src.MaterialVerified = true
				src.ValidationStatus = evidence.ValidationVerified
				src.VerifiedBy = req.ReviewerID
				src.VerifiedAt = time.Now().UTC().Format(time.RFC3339)
				src.VerificationReason = req.Reason
			}
		}
	}
	corrections := evidence.NewSourceVerifier().EnforceEvidenceIntegrity(req.Corrections, snap.Sources)
	seen := map[string]bool{}
	for _, it := range corrections {
		if it.EngineID != j.EngineID || !assessment.ValidCode(j.EngineID, it.RuleCode) || seen[it.RuleCode] || (it.Status != evidence.StatusMet && it.Status != evidence.StatusNotMet && it.Status != evidence.StatusNotApplicable) {
			WriteError(w, r, 422, ErrCodeBadRequest, "invalid or unverified correction", false)
			return
		}
		seen[it.RuleCode] = true
		c := assessment.Criterion{Code: it.RuleCode, Status: string(it.Status), Strength: it.SuggestedScore, SourceIDs: it.SourceIDs, CaseIDs: it.CaseIDs, Facts: it.Facts, Reason: it.Reason}
		if in.Request.CNV != nil {
			n, e := parseScore(it.SuggestedScore)
			if e != nil {
				WriteError(w, r, 422, ErrCodeBadRequest, "invalid fixed-point score", false)
				return
			}
			c.Score = &n
			c.Strength = ""
		}
		found := false
		for n, old := range effective.Criteria {
			if old.Code == c.Code {
				if c.Code == "PP4" && c.Status == "met" && old.Status != "met" {
					WriteError(w, r, 422, ErrCodePhenotypeUnverified, "PP4 gates require a new evaluation", false)
					return
				}
				effective.Criteria[n] = c
				found = true
			}
		}
		if !found {
			if c.Code == "PP4" && c.Status == "met" {
				WriteError(w, r, 422, ErrCodePhenotypeUnverified, "PP4 gates required", false)
				return
			}
			effective.Criteria = append(effective.Criteria, c)
		}
	}
	effective = assessment.Recompute(in.Request, effective, snap.Sources)
	result.RuleEngineResult = effective
	result.Revision++
	result.ReviewStatus = "revised_pending"
	result.ReviewerDecision = nil
	result.SettlementEligible = false
	snap.Items = assessmentItems(j.EngineID, effective.Criteria)
	snap.SnapshotID = workflow.ID("snp-")
	snap.CreatedAt = time.Now().UTC()
	snap.Overrides, _ = json.Marshal(map[string]any{"prior": json.RawMessage(snap.Overrides), "reviewer": req.ReviewerID, "corrections": corrections})
	snap.Hash, err = evidence.SnapshotHash(*snap)
	if err == nil {
		err = s.commitRevision(ctx, j, req, result, snap)
	}
	if err != nil {
		WriteError(w, r, 500, ErrCodeInternalError, "revision commit failed", false)
		return
	}
	writeJSON(w, 201, map[string]any{"assessment_id": j.JobID, "revision": result.Revision, "status": "revised_pending"})
}
func (s *Server) savedInput(ctx context.Context, j jobs.Job) (evaluationInput, error) {
	var in evaluationInput
	var enc string
	if err := s.db.QueryRowContext(ctx, `SELECT payload FROM work_inputs WHERE job_id=? AND tenant_id=?`, j.JobID, j.TenantID).Scan(&enc); err != nil {
		return in, err
	}
	raw, err := s.queue.Cipher.Open(enc, j.TenantID+":"+j.JobID+":input")
	if err != nil {
		return in, err
	}
	err = json.Unmarshal([]byte(raw), &in)
	return in, err
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
