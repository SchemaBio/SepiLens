package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pzweuj/sepilens/internal/assessment"
	"github.com/pzweuj/sepilens/internal/engine"
	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/jobs"
	"github.com/pzweuj/sepilens/internal/rules/svcv4"
	"github.com/pzweuj/sepilens/internal/strictjson"
	"github.com/pzweuj/sepilens/internal/validation"
	"github.com/pzweuj/sepilens/internal/workflow"
)

type evaluationInput struct {
	Actor          string             `json:"actor"`
	Request        assessment.Request `json:"request"`
	Sources        []evidence.Source  `json:"accepted_sources"`
	SourceGaps     []string           `json:"source_gaps"`
	InputHash      string             `json:"input_hash"`
	PolicyHash     string             `json:"policy_hash"`
	RulesetHash    string             `json:"ruleset_hash"`
	AssessmentTime time.Time          `json:"assessment_time"`
	ProfileVersion int                `json:"profile_version"`
}

func (s *Server) routesV2(r chi.Router) {
	r.Post("/evaluations", s.handleEvaluationV2)
	r.Get("/capabilities", s.handleCapabilities)
	r.Get("/rulesets", s.handleRulesets)
	r.Put("/providers/{profile_id}", s.handleProfileSave)
	r.Get("/providers/{profile_id}", s.handleProfileGet)
	r.Delete("/providers/{profile_id}", s.handleProfileDelete)
	r.Post("/providers/test", s.handleProviderTest)
	r.Delete("/tenant/data", s.handleDeleteTenantData)
	r.Get("/jobs/{job_id}", s.handleGetJob)
	r.Get("/jobs/{job_id}/result", s.handleJobResult)
	r.Get("/jobs/{job_id}/events", s.handleJobEvents)
	r.Get("/jobs/{job_id}/audit", s.handleExportJob)
	r.Post("/jobs/{job_id}/cancel", s.handleCancelJob)
	r.Delete("/jobs/{job_id}", s.handleDeleteJob)
	r.Get("/evaluations/{id}/evidence", s.handleGetEvidence)
	r.Post("/evaluations/{id}/reviews", s.handleAddReview)
}

func (s *Server) policyHash() string {
	b, _ := json.Marshal(s.cfg.SourcePolicies)
	return engine.Hash(b)
}

func (s *Server) handleEvaluationV2(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	if !p.SubmitEvidence && p.Role != "admin" {
		WriteError(w, r, 403, ErrCodeAuthFailed, "evidence submission permission required", false)
		return
	}
	limit := s.cfg.MaxEvidenceBytes
	if limit == 0 {
		limit = 8 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	b, err := io.ReadAll(r.Body)
	if err != nil {
		var size *http.MaxBytesError
		if errors.As(err, &size) {
			WriteError(w, r, 413, ErrCodeBadRequest, "evidence package too large", false)
		} else {
			WriteError(w, r, 400, ErrCodeBadRequest, "request body unavailable", false)
		}
		return
	}
	var req assessment.Request
	if strictjson.Decode(b, &req) != nil {
		WriteError(w, r, 400, ErrCodeBadRequest, "invalid or ambiguous JSON", false)
		return
	}
	req, err = assessment.Normalize(req)
	if err != nil {
		WriteError(w, r, 422, ErrCodeBadRequest, err.Error(), false)
		return
	}
	if s.queue == nil {
		WriteError(w, r, 503, ErrCodeInternalError, "worker unavailable", true)
		return
	}
	kind := "production"
	id := engine.EngineID(req.EngineID)
	if id == engine.EngineSVCv4 {
		kind = "experimental"
	} else if req.Context.VariantOrigin != "germline_confirmed" {
		kind = "evidence_only"
	} else if s.cfg.Simulation {
		kind = "simulation"
	} else if _, ok := s.approvals[id]; !ok {
		WriteError(w, r, 409, ErrCodeRulesetNotAvailable, "signed medical rule approval required", false)
		return
	}
	if req.Mode == "ai_assisted" {
		if s.cfg.AIMode != "enabled" || (kind != "production" && kind != "experimental") {
			WriteError(w, r, 409, ErrCodeRulesetNotAvailable, "AI requires explicit enablement and approved rules or experimental SVC", false)
			return
		}
		if req.Options.ProviderProfileID == "" {
			WriteError(w, r, 422, ErrCodeProviderUnavailable, "provider profile required", false)
			return
		}
	}
	content, _, err := validation.Rules(id)
	if err != nil {
		WriteError(w, r, 422, ErrCodeUnknownEngine, "unknown engine", false)
		return
	}
	rulesHash := engine.Hash(content)
	if id == engine.EngineSVCv4 {
		rulesHash = svcv4.RulesetHash()
	}
	if a, ok := s.approvals[id]; ok && kind == "production" && a.ContentHash != rulesHash {
		WriteError(w, r, 409, ErrCodeRulesetNotAvailable, "rule approval mismatch", false)
		return
	}
	input := evaluationInput{Actor: p.Actor, Request: req, RulesetHash: rulesHash, PolicyHash: s.policyHash(), InputHash: assessment.HashRequest(req)}
	if req.Mode == "ai_assisted" {
		profile, e := s.profiles.Get(r.Context(), p.Tenant, req.Options.ProviderProfileID, 0)
		if e != nil {
			WriteError(w, r, 422, ErrCodeProviderUnavailable, "provider unavailable", false)
			return
		}
		input.ProfileVersion = profile.Version
	}
	// Idempotency covers submitted data and pinned execution configuration, not clock time.
	canonical, _ := json.Marshal(input)
	requestHash := engine.Hash(canonical)
	input.AssessmentTime = time.Now().UTC()
	input.Sources, input.SourceGaps = assessment.Sources(req, p.Actor, s.cfg.SourcePolicies, input.AssessmentTime)
	payload, _ := json.Marshal(input)
	var j jobs.Job
	var replay bool
	if req.Mode == "local" {
		j, replay, err = s.queue.RunInline(r.Context(), p.Tenant, req.EngineID, kind, r.Header.Get("Idempotency-Key"), requestHash, payload)
	} else {
		j, replay, err = s.queue.Submit(r.Context(), p.Tenant, req.EngineID, kind, r.Header.Get("Idempotency-Key"), requestHash, payload)
	}
	if errors.Is(err, workflow.ErrConflict) {
		WriteError(w, r, 409, ErrCodeIdempotencyConflict, "idempotency key conflicts", false)
		return
	}
	if err != nil {
		WriteError(w, r, 500, ErrCodeInternalError, "evaluation could not be persisted", true)
		return
	}
	if req.Mode == "local" {
		j, err = s.store.GetJob(r.Context(), p.Tenant, j.JobID)
		if err != nil || j.Status != jobs.StatusSucceeded {
			WriteError(w, r, 409, ErrCodeBadRequest, "local evaluation unavailable or still running", true)
			return
		}
		var result EvaluationResultPayload
		if json.Unmarshal([]byte(j.ResultJSON), &result) != nil {
			WriteError(w, r, 500, ErrCodeInternalError, "result integrity failed", false)
			return
		}
		writeJSON(w, 200, result)
		return
	}
	writeJSON(w, 202, map[string]any{"job_id": j.JobID, "assessment_id": j.JobID, "status": j.Status, "is_replayed": replay})
}

func (s *Server) executeV2(ctx context.Context, t workflow.Task) (any, *evidence.EvidenceSnapshot, error) {
	var in evaluationInput
	if strictjson.Decode(t.Payload, &in) != nil || in.Request.SchemaVersion != assessment.SchemaVersion || in.InputHash != assessment.HashRequest(in.Request) {
		return nil, nil, errors.New("invalid pinned assessment")
	}
	if in.PolicyHash != s.policyHash() {
		return nil, nil, errors.New("pinned source policy unavailable")
	}
	content, _, err := validation.Rules(engine.EngineID(t.EngineID))
	if err != nil {
		return nil, nil, err
	}
	hash := engine.Hash(content)
	if t.EngineID == "svc_v4" {
		hash = svcv4.RulesetHash()
	}
	if hash != in.RulesetHash {
		return nil, nil, errors.New("pinned rule unavailable")
	}
	base := assessment.Evaluate(in.Request, in.Sources)
	base.Missing = append(base.Missing, in.SourceGaps...)
	result := EvaluationResultPayload{AssessmentID: t.JobID, SchemaVersion: assessment.SchemaVersion, InputHash: in.InputHash, PolicyHash: in.PolicyHash, EvaluationID: t.JobID, EngineID: t.EngineID, ResultKind: t.Kind, ClinicalUse: "requires_review", DeliveryStatus: "delivered", Revision: 1, ReviewStatus: "pending", ReviewRequired: true, RulesetHash: hash, LocalBaseline: base, RuleEngineResult: base}
	if t.Kind != "production" {
		result.ClinicalUse = "forbidden"
		if t.Kind == "experimental" {
			result.RuleEngineResult = nil
			result.AgentProposal = map[string]any{"experimental_result": base}
		}
	}
	snap := &evidence.EvidenceSnapshot{SnapshotID: workflow.ID("snp-"), EvaluationID: t.JobID, TenantID: t.TenantID, CreatedAt: in.AssessmentTime, Sources: in.Sources, Hash: ""}
	snap.HashVersion = "v2"
	snap.Annotations, _ = json.Marshal(in.Request)
	snap.Items = assessmentItems(t.EngineID, base.Criteria)
	snap.Provenance = evidence.ProvenanceMetadata{DatasetVersions: map[string]string{"input": in.InputHash, "source_policy": in.PolicyHash, "schema": assessment.SchemaVersion}, RetrievalMode: "off"}
	var problem error
	if in.Request.Mode == "ai_assisted" {
		problem = s.runAIV2(ctx, t, in, base, &result, snap)
		snap.CreatedAt = time.Now().UTC()
	}
	snap.Hash, err = evidence.SnapshotHash(*snap)
	if err != nil {
		return nil, nil, err
	}
	return result, snap, problem
}

func assessmentItems(id string, criteria []assessment.Criterion) []evidence.EvidenceItem {
	items := make([]evidence.EvidenceItem, 0, len(criteria))
	for _, c := range criteria {
		score := c.Strength
		if c.Score != nil {
			score = fmtScore(*c.Score)
		}
		items = append(items, evidence.EvidenceItem{ID: "criterion-" + c.Code, EngineID: id, RuleCode: c.Code, Status: evidence.EvidenceStatus(c.Status), SuggestedScore: score, SourceIDs: c.SourceIDs, CaseIDs: c.CaseIDs, Facts: c.Facts, Reason: c.Reason, MissingInformation: c.Missing, ReviewRequired: true})
	}
	return items
}
