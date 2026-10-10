package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pzweuj/sepilens/internal/api"
	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/retrieval"
	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
)

func TestServer_SNVEvaluation_LiveEngine(t *testing.T) {
	srv, teardown := setupTestServer(t, false) // simulation=false
	defer teardown()
	router := srv.Router()

	acmgEval := acmg2015.NewEvaluator(nil)
	srv.SetACMGEvaluator(acmgEval)

	mockRetriever := retrieval.NewPubMedRetriever(true, map[string][]evidence.Source{
		"CFTR": {
			{
				ID:               "SRC-PMID-TEST",
				Type:             evidence.SourceTypePubMed,
				PMID:             "99887766",
				Title:            "Pathogenic functional study of CFTR G542X",
				Snippet:          "Functional testing confirms loss of function and damaging impact.",
				ValidationStatus: evidence.ValidationVerified,
			},
		},
	})
	coord := retrieval.NewSearchCoordinator(retrieval.CoordinatorConfig{}, []retrieval.Retriever{mockRetriever})
	srv.SetSearchCoordinator(coord)

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
			"transcript":       "NM_000492.3",
			"hgvs_c":           "c.1624G>T",
			"hgvs_p":           "p.Gly542Ter",
			"gene":             "CFTR",
			"consequence":      "nonsense",
			"allele_frequency": rareAF,
		},
		"clinical_context": map[string]any{
			"disease_id":     "MONDO:0009061",
			"inheritance":    "AR",
			"variant_origin": "germline_confirmed",
			"sample_type":    "blood",
		},
		"options": map[string]any{
			"retrieval_mode": "targeted",
		},
	}
	body, _ := json.Marshal(snvReq)

	req := httptest.NewRequest("POST", "/v1/variants/snv/evaluations", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted for live SNV evaluation, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	jobID, ok := resp["job_id"].(string)
	if !ok || jobID == "" {
		t.Fatalf("missing job_id: %+v", resp)
	}
	if resp["classification"] != acmg2015.ClassPathogenic {
		t.Fatalf("expected Pathogenic classification, got %v", resp["classification"])
	}

	// Fetch job result
	reqRes := httptest.NewRequest("GET", "/v1/jobs/"+jobID+"/result", nil)
	reqRes.Header.Set("Authorization", authHeader)
	recRes := httptest.NewRecorder()
	router.ServeHTTP(recRes, reqRes)
	if recRes.Code != http.StatusOK {
		t.Fatalf("expected 200 for job result, got %d", recRes.Code)
	}

	var resPayload api.EvaluationResultPayload
	if err := json.Unmarshal(recRes.Body.Bytes(), &resPayload); err != nil {
		t.Fatalf("failed to parse job result: %v", err)
	}
	if resPayload.ResultKind != "production" {
		t.Fatalf("expected production result_kind, got %s", resPayload.ResultKind)
	}
	if resPayload.RuleEngineResult == nil {
		t.Fatalf("rule_engine_result must be populated for live evaluation")
	}

	// Verify evidence snapshot exists
	reqEvi := httptest.NewRequest("GET", "/v1/evaluations/"+jobID+"/evidence", nil)
	reqEvi.Header.Set("Authorization", authHeader)
	recEvi := httptest.NewRecorder()
	router.ServeHTTP(recEvi, reqEvi)
	if recEvi.Code != http.StatusOK {
		t.Fatalf("expected 200 for evidence snapshot, got %d", recEvi.Code)
	}
}

func TestServer_SNVEvaluation_WebSearchToggle(t *testing.T) {
	srv, teardown := setupTestServer(t, false)
	defer teardown()
	router := srv.Router()

	acmgEval := acmg2015.NewEvaluator(nil)
	srv.SetACMGEvaluator(acmgEval)

	mockRetriever := retrieval.NewPubMedRetriever(true, map[string][]evidence.Source{
		"CFTR": {
			{
				ID:               "SRC-PMID-TEST",
				Type:             evidence.SourceTypePubMed,
				PMID:             "99887766",
				Title:            "Pathogenic functional study of CFTR G542X",
				Snippet:          "Functional testing confirms loss of function and damaging impact.",
				ValidationStatus: evidence.ValidationVerified,
			},
		},
	})
	coord := retrieval.NewSearchCoordinator(retrieval.CoordinatorConfig{}, []retrieval.Retriever{mockRetriever})
	srv.SetSearchCoordinator(coord)

	authHeader := "Bearer test-api-key"
	rareAF := 0.00001

	// Case 1: web_search = false explicitly -> 0 web searches performed
	fBool := false
	reqPayloadOff := map[string]any{
		"engine_id": "acmg_amp_2015",
		"variant": map[string]any{
			"assembly": "GRCh38", "chrom": "chr7", "pos": 117548628, "ref": "G", "alt": "T",
			"gene": "CFTR", "consequence": "nonsense", "allele_frequency": rareAF,
		},
		"clinical_context": map[string]any{
			"disease_id": "MONDO:0009061", "inheritance": "AR",
		},
		"options": map[string]any{
			"web_search": fBool,
		},
	}
	bodyOff, _ := json.Marshal(reqPayloadOff)
	req1 := httptest.NewRequest("POST", "/v1/variants/snv/evaluations", bytes.NewReader(bodyOff))
	req1.Header.Set("Authorization", authHeader)
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d", rec1.Code)
	}
	var resp1 map[string]any
	_ = json.Unmarshal(rec1.Body.Bytes(), &resp1)
	jobID1 := resp1["job_id"].(string)

	reqRes1 := httptest.NewRequest("GET", "/v1/jobs/"+jobID1+"/result", nil)
	reqRes1.Header.Set("Authorization", authHeader)
	recRes1 := httptest.NewRecorder()
	router.ServeHTTP(recRes1, reqRes1)
	var resPayload1 api.EvaluationResultPayload
	_ = json.Unmarshal(recRes1.Body.Bytes(), &resPayload1)
	if resPayload1.Usage.WebSearches != 0 {
		t.Fatalf("expected 0 web searches when web_search=false, got %d", resPayload1.Usage.WebSearches)
	}

	// Case 2: web_search = true explicitly -> web search performed
	tBool := true
	reqPayloadOn := map[string]any{
		"engine_id": "acmg_amp_2015",
		"variant": map[string]any{
			"assembly": "GRCh38", "chrom": "chr7", "pos": 117548628, "ref": "G", "alt": "T",
			"gene": "CFTR", "consequence": "nonsense", "allele_frequency": rareAF,
		},
		"clinical_context": map[string]any{
			"disease_id": "MONDO:0009061", "inheritance": "AR",
		},
		"options": map[string]any{
			"web_search": tBool,
		},
	}
	bodyOn, _ := json.Marshal(reqPayloadOn)
	req2 := httptest.NewRequest("POST", "/v1/variants/snv/evaluations", bytes.NewReader(bodyOn))
	req2.Header.Set("Authorization", authHeader)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d", rec2.Code)
	}
	var resp2 map[string]any
	_ = json.Unmarshal(rec2.Body.Bytes(), &resp2)
	jobID2 := resp2["job_id"].(string)

	reqRes2 := httptest.NewRequest("GET", "/v1/jobs/"+jobID2+"/result", nil)
	reqRes2.Header.Set("Authorization", authHeader)
	recRes2 := httptest.NewRecorder()
	router.ServeHTTP(recRes2, reqRes2)
	var resPayload2 api.EvaluationResultPayload
	_ = json.Unmarshal(recRes2.Body.Bytes(), &resPayload2)
	if resPayload2.Usage.WebSearches != 1 {
		t.Fatalf("expected 1 web search when web_search=true, got %d", resPayload2.Usage.WebSearches)
	}
}
