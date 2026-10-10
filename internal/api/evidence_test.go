package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pzweuj/sepilens/internal/api"
	"github.com/pzweuj/sepilens/internal/evidence"
)

func TestServer_GetEvidenceSnapshot(t *testing.T) {
	srv, teardown := setupTestServer(t, true) // simulation=true
	defer teardown()
	router := srv.Router()

	authHeader := "Bearer test-api-key"

	// 1. Create a simulated evaluation job
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

	reqCreate := httptest.NewRequest("POST", "/v1/variants/snv/evaluations", bytes.NewReader(bodyBytes))
	reqCreate.Header.Set("Authorization", authHeader)
	recCreate := httptest.NewRecorder()
	router.ServeHTTP(recCreate, reqCreate)
	if recCreate.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d: %s", recCreate.Code, recCreate.Body.String())
	}

	var createResp map[string]any
	_ = json.Unmarshal(recCreate.Body.Bytes(), &createResp)
	jobID, _ := createResp["job_id"].(string)

	// 2. Query evidence snapshot for this job
	reqEvi := httptest.NewRequest("GET", "/v1/evaluations/"+jobID+"/evidence", nil)
	reqEvi.Header.Set("Authorization", authHeader)
	recEvi := httptest.NewRecorder()
	router.ServeHTTP(recEvi, reqEvi)
	if recEvi.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for evidence snapshot, got %d: %s", recEvi.Code, recEvi.Body.String())
	}

	var snp evidence.EvidenceSnapshot
	if err := json.Unmarshal(recEvi.Body.Bytes(), &snp); err != nil {
		t.Fatalf("failed to parse evidence snapshot: %v", err)
	}

	if snp.EvaluationID != jobID || snp.Hash == "" || len(snp.Sources) == 0 {
		t.Fatalf("unexpected snapshot contents: %+v", snp)
	}

	// 3. Query non-existent evaluation -> 404
	reqNonExistent := httptest.NewRequest("GET", "/v1/evaluations/non-existent-id/evidence", nil)
	reqNonExistent.Header.Set("Authorization", authHeader)
	recNonExistent := httptest.NewRecorder()
	router.ServeHTTP(recNonExistent, reqNonExistent)
	if recNonExistent.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-existent evaluation evidence, got %d", recNonExistent.Code)
	}
	var errResp api.ErrorEnvelope
	_ = json.Unmarshal(recNonExistent.Body.Bytes(), &errResp)
	if errResp.Error.Code != api.ErrCodeJobNotFound {
		t.Fatalf("expected JOB_NOT_FOUND code, got %s", errResp.Error.Code)
	}
}
