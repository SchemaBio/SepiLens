package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pzweuj/sepilens/internal/api"
	"github.com/pzweuj/sepilens/internal/rules/cnvgain2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvloss2020"
	"github.com/pzweuj/sepilens/internal/variant/cnv"
)

func TestServer_CNVEvaluation_LiveEngines(t *testing.T) {
	srv, teardown := setupTestServer(t, false)
	defer teardown()
	router := srv.Router()

	ann := cnv.NewAnnotator()
	loss := cnvloss2020.NewEngine()
	gain := cnvgain2020.NewEngine()
	srv.SetCNVEngines(loss, gain, ann)

	authHeader := "Bearer test-api-key"

	// 1. Live Loss evaluation: 17p11.2 deletion
	lossReq := map[string]any{
		"engine_id": "cnv_loss_2020",
		"cnv": map[string]any{
			"assembly":          "GRCh38",
			"chrom":             "17",
			"start":             16500000,
			"end":               20500000,
			"coordinate_system": "1-based-inclusive",
			"type":              "loss",
		},
		"clinical_context": map[string]any{
			"variant_origin": "germline_confirmed",
			"sample_type":    "blood",
		},
	}
	bodyLoss, _ := json.Marshal(lossReq)
	reqLoss := httptest.NewRequest("POST", "/v1/variants/cnv/evaluations", bytes.NewReader(bodyLoss))
	reqLoss.Header.Set("Authorization", authHeader)
	recLoss := httptest.NewRecorder()
	router.ServeHTTP(recLoss, reqLoss)
	if recLoss.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted for live loss evaluation, got %d: %s", recLoss.Code, recLoss.Body.String())
	}

	var respLoss map[string]any
	_ = json.Unmarshal(recLoss.Body.Bytes(), &respLoss)
	if respLoss["classification"] != "Pathogenic" {
		t.Fatalf("expected Pathogenic for Smith-Magenis deletion, got %v", respLoss["classification"])
	}

	// 2. Live Gain evaluation: 22q11.2 duplication
	gainReq := map[string]any{
		"engine_id": "cnv_gain_2020",
		"cnv": map[string]any{
			"assembly":          "GRCh38",
			"chrom":             "22",
			"start":             18500000,
			"end":               22000000,
			"coordinate_system": "1-based-inclusive",
			"type":              "gain",
		},
		"clinical_context": map[string]any{
			"variant_origin": "germline_confirmed",
			"sample_type":    "blood",
		},
	}
	bodyGain, _ := json.Marshal(gainReq)
	reqGain := httptest.NewRequest("POST", "/v1/variants/cnv/evaluations", bytes.NewReader(bodyGain))
	reqGain.Header.Set("Authorization", authHeader)
	recGain := httptest.NewRecorder()
	router.ServeHTTP(recGain, reqGain)
	if recGain.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted for live gain evaluation, got %d: %s", recGain.Code, recGain.Body.String())
	}

	var respGain map[string]any
	_ = json.Unmarshal(recGain.Body.Bytes(), &respGain)
	if respGain["classification"] != "Pathogenic" {
		t.Fatalf("expected Pathogenic for 22q11.2 duplication, got %v", respGain["classification"])
	}

	// 3. Check result for job
	jobID, _ := respGain["job_id"].(string)
	reqRes := httptest.NewRequest("GET", "/v1/jobs/"+jobID+"/result", nil)
	reqRes.Header.Set("Authorization", authHeader)
	recRes := httptest.NewRecorder()
	router.ServeHTTP(recRes, reqRes)
	if recRes.Code != http.StatusOK {
		t.Fatalf("expected 200 for job result, got %d", recRes.Code)
	}

	var resPayload api.EvaluationResultPayload
	_ = json.Unmarshal(recRes.Body.Bytes(), &resPayload)
	if resPayload.EngineID != "cnv_gain_2020" || resPayload.RuleEngineResult == nil {
		t.Fatalf("unexpected result payload: %+v", resPayload)
	}
}
