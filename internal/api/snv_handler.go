package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/jobs"
	"github.com/pzweuj/sepilens/internal/retrieval"
	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
)

func (s *Server) dispatchACMG2015Job(w http.ResponseWriter, r *http.Request, req *SNVEvaluationRequest, bodyBytes []byte) {
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

	// 1. Retrieval stage
	_, _ = s.store.AppendEvent(ctx, jobID, "retrieving", `{"info":"querying evidence sources"}`)

	var sources []evidence.Source
	var prov evidence.ProvenanceMetadata

	if s.coordinator != nil {
		q := retrieval.Query{
			Assembly:    req.Variant.Assembly,
			Chrom:       req.Variant.Chrom,
			Pos:         req.Variant.Pos,
			Ref:         req.Variant.Ref,
			Alt:         req.Variant.Alt,
			Gene:        req.Variant.Gene,
			DiseaseID:   req.ClinicalContext.DiseaseID,
			HGVS:        req.Variant.HGVSc,
			VariantType: "snv",
			Mode:        resolveRetrievalMode(req.Options),
		}
		prov, sources, _ = s.coordinator.Coordinate(ctx, q)
	}

	// 2. Evaluation stage
	_, _ = s.store.AppendEvent(ctx, jobID, "evaluating", `{"info":"running ACMG/AMP 2015 rule engine"}`)

	varInput := acmg2015.SNVVariantInput{
		Assembly:        req.Variant.Assembly,
		Chrom:           req.Variant.Chrom,
		Pos:             req.Variant.Pos,
		Ref:             req.Variant.Ref,
		Alt:             req.Variant.Alt,
		Transcript:      req.Variant.Transcript,
		HGVSc:           req.Variant.HGVSc,
		HGVSp:           req.Variant.HGVSp,
		Gene:            req.Variant.Gene,
		Consequence:     req.Variant.Consequence,
		AlleleFrequency: req.Variant.AlleleFrequency,
	}

	ctxInput := acmg2015.ClinicalContextInput{
		DiseaseID:     req.ClinicalContext.DiseaseID,
		Inheritance:   req.ClinicalContext.Inheritance,
		VariantOrigin: req.ClinicalContext.VariantOrigin,
		SampleType:    req.ClinicalContext.SampleType,
		HPOTerms:      req.ClinicalContext.HPOTerms,
	}

	evalOut := s.acmgEvaluator.Evaluate(varInput, ctxInput, sources)

	// 3. Evidence snapshot persistence
	var evidenceItems []evidence.EvidenceItem
	for _, ac := range evalOut.AppliedCriteria {
		evidenceItems = append(evidenceItems, evidence.EvidenceItem{
			ID:               "EVI-" + string(ac.Code),
			EngineID:         req.EngineID,
			GuidelineVersion: "2015",
			RuleCode:         string(ac.Code),
			Status:           evidence.EvidenceStatus(ac.Status),
			SuggestedScore:   string(ac.EvaluatedStrength),
			Reason:           ac.Rationale,
			SourceIDs:        ac.SourceIDs,
			SourceValidation: "verified",
			ReviewRequired:   true,
		})
	}

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

	// 4. Update Job with rule_engine_result
	resultPayload := EvaluationResultPayload{
		EvaluationID:       jobID,
		EngineID:           req.EngineID,
		ResultKind:         "production",
		ClinicalUse:        "requires_review",
		DeliveryStatus:     "delivered",
		SettlementEligible: true,
		AgentProposal:      nil,
		RuleEngineResult:   evalOut,
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
		"classification": evalOut.Classification,
	})
}
