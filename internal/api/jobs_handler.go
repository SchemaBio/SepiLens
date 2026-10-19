package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/jobs"
	"github.com/pzweuj/sepilens/internal/workflow"
)

type EvaluationResultPayload struct {
	LocalBaseline      any             `json:"local_baseline,omitempty"`
	FailureCode        string          `json:"failure_code,omitempty"`
	Revision           int             `json:"revision"`
	RulesetHash        string          `json:"ruleset_hash"`
	DatasetHash        string          `json:"dataset_hash"`
	ProfileVersion     int             `json:"profile_version,omitempty"`
	PromptVersion      string          `json:"prompt_version"`
	ReviewStatus       string          `json:"review_status"`
	EvaluationID       string          `json:"evaluation_id"`
	EngineID           string          `json:"engine_id"`
	ResultKind         string          `json:"result_kind"`
	ClinicalUse        string          `json:"clinical_use"`
	DeliveryStatus     string          `json:"delivery_status"`     // "delivered" or "failed"
	SettlementEligible bool            `json:"settlement_eligible"` // whether billing gateway can settle 1 credit
	AgentProposal      any             `json:"agent_proposal"`
	RuleEngineResult   any             `json:"rule_engine_result"`
	ReviewerDecision   *ReviewDecision `json:"reviewer_decision"`
	ReviewRequired     bool            `json:"review_required"`
	Usage              ResourceUsage   `json:"usage"`
}

type ReviewDecision struct {
	ReviewerID     string `json:"reviewer_id"`
	Decision       string `json:"decision"`
	Reason         string `json:"reason"`
	HumanOverride  bool   `json:"human_override"`
	Classification string `json:"classification"`
	ReviewedAt     string `json:"reviewed_at"`
}

type ResourceUsage struct {
	InputTokens  int   `json:"input_tokens"`
	OutputTokens int   `json:"output_tokens"`
	WebSearches  int   `json:"web_searches"`
	DurationMS   int64 `json:"duration_ms"`
}

type AddReviewRequest struct {
	Revision               int                     `json:"revision"`
	Corrections            []evidence.EvidenceItem `json:"corrections,omitempty"`
	VerifiedSources        []string                `json:"verified_sources,omitempty"`
	ReviewerID             string                  `json:"reviewer_id"`
	Decision               string                  `json:"decision"`
	HumanOverride          bool                    `json:"human_override"`
	ReviewedClassification string                  `json:"reviewed_classification"`
	Reason                 string                  `json:"reason"`
}

func (s *Server) dispatchSimulationJob(w http.ResponseWriter, r *http.Request, engineID string, bodyBytes []byte) {
	if s.queue == nil {
		WriteError(w, r, 503, ErrCodeInternalError, "worker unavailable", true)
		return
	}
	// Raw HPO text is transient; only its digest is used for idempotency.
	payload, _ := json.Marshal(envelope{DatasetHash: s.datasetHash()})
	j, replay, err := s.queue.Submit(r.Context(), GetTenantID(r.Context()), engineID, "simulation", r.Header.Get("Idempotency-Key"), computeBodyHash(bodyBytes), payload)
	if err != nil {
		status := 500
		if errors.Is(err, workflow.ErrConflict) {
			status = 409
		}
		WriteError(w, r, status, ErrCodeIdempotencyConflict, "task admission failed", false)
		return
	}
	status := 202
	if replay {
		status = 200
	}
	writeJSON(w, status, map[string]any{"job_id": j.JobID, "status": j.Status, "result_kind": j.ResultKind, "is_replayed": replay})
}

func (s *Server) handleGetJobLogic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := GetTenantID(ctx)
	jobID := chi.URLParam(r, "job_id")

	j, err := s.store.GetJob(ctx, tenantID, jobID)
	if errors.Is(err, jobs.ErrJobNotFound) {
		WriteError(w, r, http.StatusNotFound, ErrCodeJobNotFound, "job not found", false)
		return
	}
	if err != nil {
		WriteError(w, r, http.StatusInternalServerError, ErrCodeInternalError, "failed to query job", true)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(j)
}

func (s *Server) legacyJobEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := GetTenantID(ctx)
	jobID := chi.URLParam(r, "job_id")

	j, err := s.store.GetJob(ctx, tenantID, jobID)
	if errors.Is(err, jobs.ErrJobNotFound) {
		WriteError(w, r, http.StatusNotFound, ErrCodeJobNotFound, "job not found", false)
		return
	}
	if err != nil {
		WriteError(w, r, http.StatusInternalServerError, ErrCodeInternalError, "failed to query job", true)
		return
	}

	events, err := s.store.GetEvents(ctx, j.JobID, 0)
	if err != nil {
		WriteError(w, r, http.StatusInternalServerError, ErrCodeInternalError, "failed to query events", true)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteError(w, r, http.StatusInternalServerError, ErrCodeInternalError, "streaming unsupported", false)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for _, ev := range events {
		fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.Seq, ev.Stage, ev.PayloadJSON)
		flusher.Flush()
	}
}

func (s *Server) handleJobResultLogic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := GetTenantID(ctx)
	jobID := chi.URLParam(r, "job_id")

	j, err := s.store.GetJob(ctx, tenantID, jobID)
	if errors.Is(err, jobs.ErrJobNotFound) {
		WriteError(w, r, http.StatusNotFound, ErrCodeJobNotFound, "job not found", false)
		return
	}
	if err != nil {
		WriteError(w, r, http.StatusInternalServerError, ErrCodeInternalError, "failed to query job", true)
		return
	}

	if j.Status == jobs.StatusCancelled {
		WriteError(w, r, http.StatusGone, ErrCodeJobCancelled, "job was cancelled", false)
		return
	}

	if j.Status != jobs.StatusSucceeded && !(j.Status == jobs.StatusFailed && j.ResultJSON != "") {
		status := http.StatusAccepted
		if j.Status == jobs.StatusFailed {
			status = http.StatusConflict
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"job_id": j.JobID,
			"status": j.Status,
			"stage":  j.Stage,
		})
		return
	}

	var res EvaluationResultPayload
	if j.ResultKind == "legacy_unverified" {
		writeJSON(w, 200, EvaluationResultPayload{EvaluationID: j.JobID, EngineID: j.EngineID, ResultKind: "legacy_unverified", ClinicalUse: "forbidden", DeliveryStatus: "historical", ReviewRequired: true, ReviewStatus: "unverified", AgentProposal: map[string]any{"historical_result": json.RawMessage(j.ResultJSON)}})
		return
	}
	if err := json.Unmarshal([]byte(j.ResultJSON), &res); err != nil {
		WriteError(w, r, http.StatusInternalServerError, ErrCodeInternalError, "corrupted result payload", false)
		return
	}

	// Merge reviews if any
	reviews, err := s.store.GetReviews(ctx, tenantID, jobID)
	if err == nil && len(reviews) > 0 {
		latest := reviews[len(reviews)-1]
		if latest.Decision != "revise_evidence" && latest.ResultVersion == res.Revision {
			res.ReviewerDecision = &ReviewDecision{
				ReviewerID:     latest.ReviewerID,
				Decision:       latest.Decision,
				Reason:         latest.Reason,
				HumanOverride:  latest.HumanOverride,
				Classification: latest.ReviewedClassification,
				ReviewedAt:     latest.CreatedAt.Format("2006-01-02T15:04:05Z"),
			}
			res.ReviewStatus = latest.Decision
			res.ReviewRequired = latest.Decision == "reject"
			if latest.Decision == "reject" {
				res.SettlementEligible = false
			} else if latest.Decision == "accept_engine_result" || latest.Decision == "override" {
				res.SettlementEligible = s.reviewSettlementEligible(ctx, j, res)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) handleCancelJobLogic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := GetTenantID(ctx)
	jobID := chi.URLParam(r, "job_id")

	err := s.store.CancelJob(ctx, tenantID, jobID)
	if errors.Is(err, jobs.ErrJobNotFound) {
		WriteError(w, r, http.StatusNotFound, ErrCodeJobNotFound, "job not found", false)
		return
	}
	if errors.Is(err, jobs.ErrJobAlreadyFinished) {
		WriteError(w, r, http.StatusConflict, ErrCodeBadRequest, "job is already in a terminal state", false)
		return
	}
	if err != nil {
		WriteError(w, r, http.StatusInternalServerError, ErrCodeInternalError, "failed to cancel job", true)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "cancelled", "job_id": jobID})
}

func (s *Server) handleAddReviewLogic(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, "reviewer", "admin") {
		return
	}
	ctx := r.Context()
	tenantID := GetTenantID(ctx)
	evalID := chi.URLParam(r, "id")

	j, err := s.store.GetJob(ctx, tenantID, evalID)
	if errors.Is(err, jobs.ErrJobNotFound) {
		WriteError(w, r, http.StatusNotFound, ErrCodeJobNotFound, "evaluation job not found", false)
		return
	}
	if err != nil {
		WriteError(w, r, http.StatusInternalServerError, ErrCodeInternalError, "failed to query evaluation", true)
		return
	}

	var req AddReviewRequest
	if !decode(w, r, &req) {
		return
	}
	var current EvaluationResultPayload
	if json.Unmarshal([]byte(j.ResultJSON), &current) != nil {
		WriteError(w, r, 409, ErrCodeBadRequest, "valid result required", false)
		return
	}
	if req.Revision == 0 {
		req.Revision = 1
	}
	if current.Revision != req.Revision {
		WriteError(w, r, 409, ErrCodeBadRequest, "result revision changed", false)
		return
	}

	p := principal(r)
	if req.ReviewerID != "" && req.ReviewerID != p.Actor {
		WriteError(w, r, 403, ErrCodeAuthFailed, "reviewer identity mismatch", false)
		return
	}
	req.ReviewerID = p.Actor
	if j.Status != jobs.StatusSucceeded {
		WriteError(w, r, 409, ErrCodeBadRequest, "completed evaluation required", false)
		return
	}
	if req.Decision != "accept_engine_result" && req.Decision != "reject" && req.Decision != "override" && req.Decision != "revise_evidence" {
		WriteError(w, r, 422, ErrCodeBadRequest, "invalid review decision", false)
		return
	}
	if strings.TrimSpace(req.Reason) == "" {
		WriteError(w, r, 422, ErrCodeBadRequest, "review reason required", false)
		return
	}

	// Medical constraint: In simulation or uncalculated state, rule engine produced no result
	if req.Decision == "accept_engine_result" {
		var res EvaluationResultPayload
		_ = json.Unmarshal([]byte(j.ResultJSON), &res)
		if j.ResultKind != "production" || res.RuleEngineResult == nil {
			WriteError(w, r, 409, ErrCodeRulesetNotAvailable, "valid production rule result required", false)
			return
		}
	}
	if req.Decision == "override" {
		req.HumanOverride = true
	}
	if req.Decision == "revise_evidence" {
		s.reviseEvidence(w, r, j, req)
		return
	}
	if req.HumanOverride && !validClassification(req.ReviewedClassification) {
		WriteError(w, r, 422, ErrCodeBadRequest, "invalid reviewed classification", false)
		return
	}

	if req.HumanOverride {
		if strings.TrimSpace(req.Reason) == "" {
			WriteError(w, r, http.StatusUnprocessableEntity, ErrCodeBadRequest, "reason is required for human override", false)
			return
		}
		if strings.TrimSpace(req.ReviewedClassification) == "" {
			WriteError(w, r, http.StatusUnprocessableEntity, ErrCodeBadRequest, "reviewed_classification is required for human override", false)
			return
		}
	}

	review := jobs.ReviewRecord{
		ExpectedUpdatedAt:      j.UpdatedAt,
		ResultVersion:          req.Revision,
		EvaluationID:           evalID,
		TenantID:               tenantID,
		ReviewerID:             req.ReviewerID,
		Decision:               req.Decision,
		Reason:                 req.Reason,
		HumanOverride:          req.HumanOverride,
		ReviewedClassification: req.ReviewedClassification,
	}

	if err := s.store.AddReview(ctx, review); err != nil {
		if errors.Is(err, jobs.ErrJobAlreadyFinished) {
			WriteError(w, r, 409, ErrCodeBadRequest, "result revision changed", false)
			return
		}
		WriteError(w, r, http.StatusInternalServerError, ErrCodeInternalError, "failed to store review", true)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"evaluation_id": evalID,
		"status":        "review_recorded",
		"decision":      req.Decision,
	})
	_ = j
}
