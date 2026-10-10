package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/jobs"
	"github.com/pzweuj/sepilens/internal/retrieval"
	"github.com/pzweuj/sepilens/internal/rules/cnvgain2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvloss2020"
	"github.com/pzweuj/sepilens/internal/variant/cnv"
)

func (s *Server) dispatchCNVJob(w http.ResponseWriter, r *http.Request, req *CNVEvaluationRequest, bodyBytes []byte) {
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
		EngineID:       req.EngineID,
		Status:         jobs.StatusQueued,
		ResultKind:     "production",
		RequestHash:    bodyHash,
		IdempotencyKey: idempotencyKey,
		Stage:          "queued",
		Progress:       0,
	}

	if err := s.store.CreateJob(ctx, newJob); err != nil {
		WriteError(w, r, http.StatusInternalServerError, ErrCodeInternalError, "failed to create job", true)
		return
	}

	_, _ = s.store.AppendEvent(ctx, jobID, "queued", `{"info":"job placed in queue"}`)

	// 1. Retrieval & Annotation
	_, _ = s.store.AppendEvent(ctx, jobID, "annotating", `{"info":"annotating CNV genomic coordinates and dosage sensitivity"}`)

	cnvInput := cnv.CNVInput{
		Assembly: req.CNV.Assembly,
		Chrom:    req.CNV.Chrom,
		Start:    req.CNV.Start,
		End:      req.CNV.End,
		Type:     req.CNV.Type,
	}
	annotated := s.cnvAnnotator.Annotate(cnvInput)

	var sources []evidence.Source
	var prov evidence.ProvenanceMetadata
	if s.coordinator != nil {
		q := retrieval.Query{
			Assembly:    req.CNV.Assembly,
			Chrom:       req.CNV.Chrom,
			Start:       req.CNV.Start,
			End:         req.CNV.End,
			VariantType: "cnv_" + req.CNV.Type,
			Mode:        resolveRetrievalMode(req.Options),
		}
		prov, sources, _ = s.coordinator.Coordinate(ctx, q)
	}

	// 2. Execute respective engine
	_, _ = s.store.AppendEvent(ctx, jobID, "evaluating", fmt.Sprintf(`{"info":"evaluating %s rubric"}`, req.EngineID))

	var classification string
	var totalScore string
	var ruleEngineResult any
	var evidenceItems []evidence.EvidenceItem

	if req.EngineID == "cnv_loss_2020" {
		lossInput := cnvloss2020.LossEvaluationInput{
			CNV:         annotated,
			Inheritance: "unknown",
		}
		lossRes := s.cnvLossEngine.Evaluate(lossInput)
		classification = lossRes.Classification
		totalScore = lossRes.TotalScore
		ruleEngineResult = lossRes

		for _, cl := range lossRes.Clauses {
			scoreStr := fmt.Sprintf("%.2f", float64(cl.Score)/100.0)
			evidenceItems = append(evidenceItems, evidence.EvidenceItem{
				ID:               "EVI-LOSS-" + cl.Code,
				EngineID:         "cnv_loss_2020",
				GuidelineVersion: "2020",
				RuleCode:         cl.Code,
				Status:           evidence.StatusMet,
				SuggestedScore:   scoreStr,
				ScoreApplied:     &scoreStr,
				Reason:           cl.Reason,
				SourceValidation: "verified",
				ReviewRequired:   true,
			})
		}
	} else if req.EngineID == "cnv_gain_2020" {
		gainInput := cnvgain2020.GainEvaluationInput{
			CNV:                    annotated,
			TandemOrientationKnown: true,
			Inheritance:            "unknown",
		}
		gainRes := s.cnvGainEngine.Evaluate(gainInput)
		classification = gainRes.Classification
		totalScore = gainRes.TotalScore
		ruleEngineResult = gainRes

		for _, cl := range gainRes.Clauses {
			scoreStr := fmt.Sprintf("%.2f", float64(cl.Score)/100.0)
			evidenceItems = append(evidenceItems, evidence.EvidenceItem{
				ID:               "EVI-GAIN-" + cl.Code,
				EngineID:         "cnv_gain_2020",
				GuidelineVersion: "2020",
				RuleCode:         cl.Code,
				Status:           evidence.StatusMet,
				SuggestedScore:   scoreStr,
				ScoreApplied:     &scoreStr,
				Reason:           cl.Reason,
				SourceValidation: "verified",
				ReviewRequired:   true,
			})
		}
	}

	// 3. Persist Evidence Snapshot
	hash, _ := evidence.ComputeSnapshotHash(sources, evidenceItems, prov)
	snapshot := evidence.EvidenceSnapshot{
		SnapshotID:   "snp-" + jobID,
		EvaluationID: jobID,
		TenantID:     tenantID,
		Hash:         hash,
		Sources:      sources,
		Items:        evidenceItems,
		Provenance:   prov,
		CreatedAt:    time.Now().UTC(),
	}
	if s.evidenceStore != nil {
		_ = s.evidenceStore.SaveSnapshot(ctx, snapshot)
	}

	// 4. Update Job Result
	resultPayload := EvaluationResultPayload{
		EvaluationID:       jobID,
		EngineID:           req.EngineID,
		ResultKind:         "production",
		ClinicalUse:        "requires_review",
		DeliveryStatus:     "delivered",
		SettlementEligible: true,
		AgentProposal:      nil,
		RuleEngineResult:   ruleEngineResult,
		ReviewerDecision:   nil,
		ReviewRequired:     true,
		Usage: ResourceUsage{
			InputTokens:  0,
			OutputTokens: 0,
			WebSearches:  prov.RequestsCount,
			DurationMS:   prov.DurationMS,
		},
	}
	resBytes, _ := json.Marshal(resultPayload)
	resStr := string(resBytes)

	_ = s.store.UpdateJobStatus(ctx, jobID, jobs.StatusSucceeded, "completed", 100, &resStr, nil)
	_, _ = s.store.AppendEvent(ctx, jobID, "completed", `{"status":"succeeded"}`)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"job_id":         jobID,
		"status":         "succeeded",
		"result_kind":    "production",
		"classification": classification,
		"total_score":    totalScore,
	})
}
