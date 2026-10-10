package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pzweuj/sepilens/internal/api"
)

func TestServer_SNVAdmissionAndGates(t *testing.T) {
	srv, teardown := setupTestServer(t, false) // simulation=false
	defer teardown()
	router := srv.Router()

	authHeader := "Bearer test-api-key"

	// 1. Missing MOI / clinical context -> 422 NEEDS_CLINICAL_CONTEXT
	snvReqNoMOI := map[string]any{
		"engine_id": "acmg_amp_2015",
		"variant": map[string]any{
			"assembly": "GRCh38",
			"chrom":    "chr7",
			"pos":      117199644,
			"ref":      "ATCT",
			"alt":      "A",
			"gene":     "CFTR",
		},
		"clinical_context": map[string]any{
			"disease_id":  "MONDO:0009061",
			"inheritance": "", // empty MOI
		},
	}
	body, _ := json.Marshal(snvReqNoMOI)
	req := httptest.NewRequest("POST", "/v1/variants/snv/evaluations", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing MOI, got %d", rec.Code)
	}
	var errResp api.ErrorEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp.Error.Code != api.ErrCodeNeedsClinicalContext {
		t.Fatalf("expected NEEDS_CLINICAL_CONTEXT, got %s", errResp.Error.Code)
	}

	// 2. SVC v4 is permanently disabled -> 409 RULESET_NOT_AVAILABLE
	snvReqSVCv4 := map[string]any{
		"engine_id": "svc_v4",
		"variant": map[string]any{
			"assembly": "GRCh38",
			"chrom":    "chr7",
			"pos":      117199644,
			"ref":      "ATCT",
			"alt":      "A",
			"gene":     "CFTR",
		},
		"clinical_context": map[string]any{
			"disease_id":  "MONDO:0009061",
			"inheritance": "AR",
		},
	}
	body, _ = json.Marshal(snvReqSVCv4)
	req = httptest.NewRequest("POST", "/v1/variants/snv/evaluations", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for svc_v4, got %d", rec.Code)
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp.Error.Code != api.ErrCodeRulesetNotAvailable {
		t.Fatalf("expected RULESET_NOT_AVAILABLE for svc_v4, got %s", errResp.Error.Code)
	}

	// 3. ACMG 2015 when simulation is disabled -> 409 RULESET_NOT_AVAILABLE
	snvReqACMG := map[string]any{
		"engine_id": "acmg_amp_2015",
		"variant": map[string]any{
			"assembly": "GRCh38",
			"chrom":    "chr7",
			"pos":      117199644,
			"ref":      "ATCT",
			"alt":      "A",
			"gene":     "CFTR",
		},
		"clinical_context": map[string]any{
			"disease_id":  "MONDO:0009061",
			"inheritance": "AR",
		},
	}
	body, _ = json.Marshal(snvReqACMG)
	req = httptest.NewRequest("POST", "/v1/variants/snv/evaluations", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for live acmg_amp_2015 in cycle 0, got %d", rec.Code)
	}

	// 4. Invalid genome assembly -> 422 ASSEMBLY_MISMATCH
	snvReqBadAsm := map[string]any{
		"engine_id": "acmg_amp_2015",
		"variant": map[string]any{
			"assembly": "NCBI36",
			"chrom":    "chr7",
			"pos":      117199644,
			"ref":      "ATCT",
			"alt":      "A",
			"gene":     "CFTR",
		},
		"clinical_context": map[string]any{
			"disease_id":  "MONDO:0009061",
			"inheritance": "AR",
		},
	}
	body, _ = json.Marshal(snvReqBadAsm)
	req = httptest.NewRequest("POST", "/v1/variants/snv/evaluations", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for invalid assembly, got %d", rec.Code)
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp.Error.Code != api.ErrCodeAssemblyMismatch {
		t.Fatalf("expected ASSEMBLY_MISMATCH, got %s", errResp.Error.Code)
	}
}

func TestServer_CNVAdmissionAndGates(t *testing.T) {
	srv, teardown := setupTestServer(t, false)
	defer teardown()
	router := srv.Router()

	authHeader := "Bearer test-api-key"

	// 1. cnv_loss_2020 with type=gain -> 422 INVALID_VARIANT
	cnvMismatch := map[string]any{
		"engine_id": "cnv_loss_2020",
		"cnv": map[string]any{
			"assembly":          "GRCh38",
			"chrom":             "7",
			"start":             10000000,
			"end":               12000000,
			"coordinate_system": "1-based-inclusive",
			"type":              "gain", // mismatch!
		},
		"clinical_context": map[string]any{
			"variant_origin": "germline_confirmed",
		},
	}
	body, _ := json.Marshal(cnvMismatch)
	req := httptest.NewRequest("POST", "/v1/variants/cnv/evaluations", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for loss/gain mismatch, got %d", rec.Code)
	}

	// 2. Somatic confirmed CNV origin -> 422 UNSUPPORTED_VARIANT_ORIGIN
	cnvSomatic := map[string]any{
		"engine_id": "cnv_loss_2020",
		"cnv": map[string]any{
			"assembly":          "GRCh38",
			"chrom":             "7",
			"start":             10000000,
			"end":               12000000,
			"coordinate_system": "1-based-inclusive",
			"type":              "loss",
		},
		"clinical_context": map[string]any{
			"variant_origin": "somatic_confirmed", // somatic prohibited in constitutional CNV
		},
	}
	body, _ = json.Marshal(cnvSomatic)
	req = httptest.NewRequest("POST", "/v1/variants/cnv/evaluations", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for somatic CNV, got %d", rec.Code)
	}
	var errResp api.ErrorEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp.Error.Code != api.ErrCodeUnsupportedVariantOrigin {
		t.Fatalf("expected UNSUPPORTED_VARIANT_ORIGIN, got %s", errResp.Error.Code)
	}

	// 3. Missing coordinate system -> 422 INVALID_COORDINATE_SYSTEM
	cnvNoCoord := map[string]any{
		"engine_id": "cnv_loss_2020",
		"cnv": map[string]any{
			"assembly": "GRCh38",
			"chrom":    "7",
			"start":    10000000,
			"end":      12000000,
			"type":     "loss",
		},
		"clinical_context": map[string]any{
			"variant_origin": "germline_confirmed",
		},
	}
	body, _ = json.Marshal(cnvNoCoord)
	req = httptest.NewRequest("POST", "/v1/variants/cnv/evaluations", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing coordinate system, got %d", rec.Code)
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp.Error.Code != api.ErrCodeInvalidCoordinateSystem {
		t.Fatalf("expected INVALID_COORDINATE_SYSTEM, got %s", errResp.Error.Code)
	}
}

func TestServer_Simulation_Idempotency_SSE_And_Reviews(t *testing.T) {
	srv, teardown := setupTestServer(t, true) // simulation=true
	defer teardown()
	router := srv.Router()

	authHeader := "Bearer test-api-key"

	snvPayload := map[string]any{
		"engine_id": "acmg_amp_2015",
		"variant": map[string]any{
			"assembly": "GRCh38",
			"chrom":    "chr7",
			"pos":      117199644,
			"ref":      "ATCT",
			"alt":      "A",
			"gene":     "CFTR",
		},
		"clinical_context": map[string]any{
			"disease_id":  "MONDO:0009061",
			"inheritance": "AR",
		},
	}
	bodyBytes, _ := json.Marshal(snvPayload)

	// 1. Create simulated job with Idempotency-Key
	reqCreate := httptest.NewRequest("POST", "/v1/variants/snv/evaluations", bytes.NewReader(bodyBytes))
	reqCreate.Header.Set("Authorization", authHeader)
	reqCreate.Header.Set("Idempotency-Key", "eval-idem-001")
	recCreate := httptest.NewRecorder()
	router.ServeHTTP(recCreate, reqCreate)
	if recCreate.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d: %s", recCreate.Code, recCreate.Body.String())
	}

	var createResp map[string]any
	_ = json.Unmarshal(recCreate.Body.Bytes(), &createResp)
	jobID, ok := createResp["job_id"].(string)
	if !ok || jobID == "" {
		t.Fatalf("missing job_id in creation response: %+v", createResp)
	}

	// 2. Replay same request with same Idempotency-Key -> 200 with is_replayed
	reqReplay := httptest.NewRequest("POST", "/v1/variants/snv/evaluations", bytes.NewReader(bodyBytes))
	reqReplay.Header.Set("Authorization", authHeader)
	reqReplay.Header.Set("Idempotency-Key", "eval-idem-001")
	recReplay := httptest.NewRecorder()
	router.ServeHTTP(recReplay, reqReplay)
	if recReplay.Code != http.StatusOK {
		t.Fatalf("expected 200 on replay, got %d", recReplay.Code)
	}

	// 3. Different request with same Idempotency-Key -> 409 IDEMPOTENCY_CONFLICT
	diffPayload := map[string]any{
		"engine_id": "acmg_amp_2015",
		"variant": map[string]any{
			"assembly": "GRCh38",
			"chrom":    "chr7",
			"pos":      117199644,
			"ref":      "ATCT",
			"alt":      "A",
			"gene":     "CFTR2", // modified!
		},
		"clinical_context": map[string]any{
			"disease_id":  "MONDO:0009061",
			"inheritance": "AR",
		},
	}
	diffBytes, _ := json.Marshal(diffPayload)
	reqConflict := httptest.NewRequest("POST", "/v1/variants/snv/evaluations", bytes.NewReader(diffBytes))
	reqConflict.Header.Set("Authorization", authHeader)
	reqConflict.Header.Set("Idempotency-Key", "eval-idem-001")
	recConflict := httptest.NewRecorder()
	router.ServeHTTP(recConflict, reqConflict)
	if recConflict.Code != http.StatusConflict {
		t.Fatalf("expected 409 conflict, got %d", recConflict.Code)
	}

	// 4. SSE events stream
	reqEvents := httptest.NewRequest("GET", "/v1/jobs/"+jobID+"/events", nil)
	reqEvents.Header.Set("Authorization", authHeader)
	recEvents := httptest.NewRecorder()
	router.ServeHTTP(recEvents, reqEvents)
	if recEvents.Code != http.StatusOK {
		t.Fatalf("expected 200 for SSE events, got %d", recEvents.Code)
	}
	eventsStr := recEvents.Body.String()
	if !strings.Contains(eventsStr, "event: queued") || !strings.Contains(eventsStr, "event: completed") {
		t.Fatalf("expected queued and completed events, got: %s", eventsStr)
	}

	// 5. GET job result
	reqResult := httptest.NewRequest("GET", "/v1/jobs/"+jobID+"/result", nil)
	reqResult.Header.Set("Authorization", authHeader)
	recResult := httptest.NewRecorder()
	router.ServeHTTP(recResult, reqResult)
	if recResult.Code != http.StatusOK {
		t.Fatalf("expected 200 for job result, got %d", recResult.Code)
	}
	var res api.EvaluationResultPayload
	_ = json.Unmarshal(recResult.Body.Bytes(), &res)
	if res.ClinicalUse != "forbidden" {
		t.Fatalf("expected clinical_use=forbidden, got %s", res.ClinicalUse)
	}
	if res.RuleEngineResult != nil {
		t.Fatalf("rule_engine_result must be nil for simulated/unimplemented rule")
	}

	// 6. Review: attempting accept_engine_result on empty engine output fails -> 409
	revAccept := map[string]any{
		"reviewer_id": "dr-smith",
		"decision":    "accept_engine_result",
	}
	revAcceptBytes, _ := json.Marshal(revAccept)
	reqRevAccept := httptest.NewRequest("POST", "/v1/evaluations/"+jobID+"/reviews", bytes.NewReader(revAcceptBytes))
	reqRevAccept.Header.Set("Authorization", authHeader)
	recRevAccept := httptest.NewRecorder()
	router.ServeHTTP(recRevAccept, reqRevAccept)
	if recRevAccept.Code != http.StatusConflict {
		t.Fatalf("expected 409 when accepting empty engine result, got %d", recRevAccept.Code)
	}

	// 7. Review: Human override with reason succeeds
	revOverride := map[string]any{
		"reviewer_id":             "dr-smith",
		"decision":                "override",
		"human_override":          true,
		"reviewed_classification": "Pathogenic",
		"reason":                  "Confirmed by functional patch clamp test",
	}
	revOverrideBytes, _ := json.Marshal(revOverride)
	reqRevOverride := httptest.NewRequest("POST", "/v1/evaluations/"+jobID+"/reviews", bytes.NewReader(revOverrideBytes))
	reqRevOverride.Header.Set("Authorization", authHeader)
	recRevOverride := httptest.NewRecorder()
	router.ServeHTTP(recRevOverride, reqRevOverride)
	if recRevOverride.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for human override review, got %d: %s", recRevOverride.Code, recRevOverride.Body.String())
	}

	// 8. Result now reflects reviewer_decision, while rule_engine_result remains nil!
	reqResultAfter := httptest.NewRequest("GET", "/v1/jobs/"+jobID+"/result", nil)
	reqResultAfter.Header.Set("Authorization", authHeader)
	recResultAfter := httptest.NewRecorder()
	router.ServeHTTP(recResultAfter, reqResultAfter)
	var resAfter api.EvaluationResultPayload
	_ = json.Unmarshal(recResultAfter.Body.Bytes(), &resAfter)
	if resAfter.RuleEngineResult != nil {
		t.Fatalf("rule_engine_result must still be nil")
	}
	if resAfter.ReviewerDecision == nil || resAfter.ReviewerDecision.Classification != "Pathogenic" {
		t.Fatalf("expected reviewer_decision Pathogenic, got %+v", resAfter.ReviewerDecision)
	}
}
