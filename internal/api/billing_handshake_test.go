package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pzweuj/sepilens/internal/api"
	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
)

func TestServer_BillingHandshakeFields(t *testing.T) {
	srv, teardown := setupTestServer(t, false)
	defer teardown()
	router := srv.Router()

	acmgEval := acmg2015.NewEvaluator(nil)
	srv.SetACMGEvaluator(acmgEval)

	authHeader := "Bearer test-api-key"

	rareAF := 0.00001
	snvReq := map[string]any{
		"engine_id": "acmg_amp_2015",
		"variant": map[string]any{
			"assembly":         "GRCh38",
			"chrom":            "chr7",
			"pos":              117548628,
			"ref":              "G",
			"alt":              "T",
			"gene":             "CFTR",
			"consequence":      "nonsense",
			"allele_frequency": rareAF,
		},
		"clinical_context": map[string]any{
			"disease_id":  "MONDO:0009061",
			"inheritance": "AR",
		},
	}
	body, _ := json.Marshal(snvReq)

	req := httptest.NewRequest("POST", "/v1/variants/snv/evaluations", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d", rec.Code)
	}

	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	jobID, _ := resp["job_id"].(string)

	// Fetch result and assert billing settlement fields
	reqRes := httptest.NewRequest("GET", "/v1/jobs/"+jobID+"/result", nil)
	reqRes.Header.Set("Authorization", authHeader)
	recRes := httptest.NewRecorder()
	router.ServeHTTP(recRes, reqRes)
	if recRes.Code != http.StatusOK {
		t.Fatalf("expected 200 for job result, got %d", recRes.Code)
	}

	var payload api.EvaluationResultPayload
	if err := json.Unmarshal(recRes.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}

	if payload.DeliveryStatus != "delivered" {
		t.Fatalf("expected delivery_status=delivered, got %s", payload.DeliveryStatus)
	}
	if !payload.SettlementEligible {
		t.Fatalf("expected settlement_eligible=true for successful delivery")
	}
}
