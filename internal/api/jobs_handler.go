package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/jobs"
)

type EvaluationResultPayload struct {
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
	ReviewerID             string `json:"reviewer_id"`
	Decision               string `json:"decision"`
	HumanOverride          bool   `json:"human_override"`
	ReviewedClassification string `json:"reviewed_classification"`
	Reason                 string `json:"reason"`
}

func (s *Server) dispatchSimulationJob(w http.ResponseWriter, r *http.Request, engineID string, bodyBytes []byte) {
	ctx := r.Context()
	tenantID := GetTenantID(ctx)
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	bodyHash := computeBodyHash(bodyBytes)

	if idempotencyKey != "" {
		existing, err := s.store.FindByIdempotency(ctx, tenantID, idempotencyKey)
		if err != nil {
			WriteError(w, r, http.StatusInternalServerError, ErrCodeInternalError, "failed to check idempotency", true)
			return
		}
		if existing != nil {
			if existing.RequestHash != bodyHash {
				WriteError(w, r, http.StatusConflict, ErrCodeIdempotencyConflict, "idempotency key matches a previous request with different parameters", false)
				return
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"job_id":      existing.JobID,
				"status":      existing.Status,
				"result_kind": existing.ResultKind,
				"is_replayed": true,
			})
			return
		}
	}

	var b [8]byte
	_, _ = rand.Read(b[:])
	jobID := "job-" + hex.EncodeToString(b[:])

	newJob := jobs.Job{
		JobID:          jobID,
		TenantID:       tenantID,
		EngineID:       engineID,
		Status:         jobs.StatusQueued,
		ResultKind:     "simulation",
		RequestHash:    bodyHash,
		IdempotencyKey: idempotencyKey,
		Stage:          "queued",
		Progress:       0,
	}

	if err := s.store.CreateJob(ctx, newJob); err != nil {
		WriteError(w, r, http.StatusInternalServerError, ErrCodeInternalError, "failed to create job", true)
		return
	}

	// Produce events and advance simulation to completion
	_, _ = s.store.AppendEvent(ctx, jobID, "queued", `{"info":"job placed in queue"}`)
	_, _ = s.store.AppendEvent(ctx, jobID, "running", `{"step":"simulated evaluation execution"}`)

	simResult := EvaluationResultPayload{
		EvaluationID:       jobID,
		EngineID:           engineID,
		ResultKind:         "simulation",
		ClinicalUse:        "forbidden",
		DeliveryStatus:     "delivered",
		SettlementEligible: true,
		AgentProposal:      nil,
		RuleEngineResult:   nil,
		ReviewerDecision:   nil,
		ReviewRequired:     true,
		Usage: ResourceUsage{
			InputTokens:  0,
			OutputTokens: 0,
			WebSearches:  0,
			DurationMS:   15,
		},
	}
	resBytes, _ := json.Marshal(simResult)
	resStr := string(resBytes)

	_ = s.store.UpdateJobStatus(ctx, jobID, jobs.StatusSucceeded, "completed", 100, &resStr, nil)
	_, _ = s.store.AppendEvent(ctx, jobID, "completed", `{"status":"succeeded"}`)

	// If evidence store is available, persist initial evidence snapshot
	if s.evidenceStore != nil {
		sources := []evidence.Source{
			{
				ID:               "SRC-CLINVAR-" + jobID,
				Type:             evidence.SourceTypeClinVar,
				Title:            "ClinVar Variant Assessment Reference",
				ClinVarID:        "VCV0000001",
				ValidationStatus: evidence.ValidationVerified,
				RetrievedAt:      time.Now().UTC().Format(time.RFC3339),
			},
		}
		items := []evidence.EvidenceItem{
			{
				ID:               "EVI-SIM-01",
				EngineID:         engineID,
				RuleCode:         "SIM",
				Status:           evidence.StatusProposedMet,
				Reason:           "Simulated candidate evidence placeholder",
				SourceIDs:        []string{"SRC-CLINVAR-" + jobID},
				SourceValidation: "verified",
				ReviewRequired:   true,
			},
		}
		prov := evidence.ProvenanceMetadata{
			RetrievalMode:      "targeted",
			RetrievalTimestamp: time.Now().UTC().Format(time.RFC3339),
			TokensUsed:         0,
			RequestsCount:      1,
			DurationMS:         15,
		}
		hash, _ := evidence.ComputeSnapshotHash(sources, items, prov)
		snapshot := evidence.EvidenceSnapshot{
			SnapshotID:   "snp-" + jobID,
			EvaluationID: jobID,
			TenantID:     tenantID,
			Hash:         hash,
			Sources:      sources,
			Items:        items,
			Provenance:   prov,
			CreatedAt:    time.Now().UTC(),
		}
		_ = s.evidenceStore.SaveSnapshot(ctx, snapshot)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"job_id":      jobID,
		"status":      "succeeded",
		"result_kind": "simulation",
	})
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

func (s *Server) handleJobEventsLogic(w http.ResponseWriter, r *http.Request) {
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

	if j.Status != jobs.StatusSucceeded {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"job_id": j.JobID,
			"status": j.Status,
			"stage":  j.Stage,
		})
		return
	}

	var res EvaluationResultPayload
	if err := json.Unmarshal([]byte(j.ResultJSON), &res); err != nil {
		WriteError(w, r, http.StatusInternalServerError, ErrCodeInternalError, "corrupted result payload", false)
		return
	}

	// Merge reviews if any
	reviews, err := s.store.GetReviews(ctx, tenantID, jobID)
	if err == nil && len(reviews) > 0 {
		latest := reviews[len(reviews)-1]
		res.ReviewerDecision = &ReviewDecision{
			ReviewerID:     latest.ReviewerID,
			Decision:       latest.Decision,
			Reason:         latest.Reason,
			HumanOverride:  latest.HumanOverride,
			Classification: latest.ReviewedClassification,
			ReviewedAt:     latest.CreatedAt.Format("2006-01-02T15:04:05Z"),
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, r, http.StatusBadRequest, ErrCodeBadRequest, "invalid review payload", false)
		return
	}

	if req.ReviewerID == "" {
		WriteError(w, r, http.StatusBadRequest, ErrCodeBadRequest, "reviewer_id is required", false)
		return
	}

	// Medical constraint: In simulation or uncalculated state, rule engine produced no result
	if req.Decision == "accept_engine_result" {
		WriteError(w, r, http.StatusConflict, ErrCodeRulesetNotAvailable, "cannot accept rule engine result because rule engine produced no result in this evaluation", false)
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
		EvaluationID:           evalID,
		TenantID:               tenantID,
		ReviewerID:             req.ReviewerID,
		Decision:               req.Decision,
		Reason:                 req.Reason,
		HumanOverride:          req.HumanOverride,
		ReviewedClassification: req.ReviewedClassification,
	}

	if err := s.store.AddReview(ctx, review); err != nil {
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
