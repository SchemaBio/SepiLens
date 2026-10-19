package api_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pzweuj/sepilens/internal/api"
	"github.com/pzweuj/sepilens/internal/engine"
	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
)

func annotationInput() string {
	return `{
		"engine_id":"acmg_amp_2015",
		"mode":"local",
		"variant":{"assembly":"GRCh38","chrom":"7","pos":100,"ref":"A","alt":"G","gene":"CFTR","transcript":"NM_000492.4","consequence":"missense_variant","hgvs_p":"p.Phe508del"},
		"clinical_context":{"variant_origin":"germline_confirmed","sample_type":"blood","inheritance":"AR","disease_id":"MONDO:0009061"},
		"annotations":{
			"source":"caller_supplied",
			"gnomad":{"version":"v4.1","assembly":"GRCh38","status":"found","faf95":0.00001,"popmax_af":0.00002,"allele_number":1200000,"coverage_adequate":true},
			"alphamissense":{"version":"2023","status":"found","score":0.92,"class":"likely_pathogenic","transcript":"NM_000492.4"},
			"gencc":{"version":"2026-09","status":"found","gene":"CFTR","disease_id":"MONDO:0009061","classification":"Definitive","moi":"AR"}
		},
		"options":{"retrieval_mode":"off","combination_method":"svi_points"}
	}`
}

func TestLocalAnnotationEvaluation(t *testing.T) {
	s, closeServer := setupTestServer(t, false)
	defer closeServer()
	s.SetACMGEvaluator(acmg2015.NewEvaluator(nil))
	// Synthetic approval confined to this test; it is not a medical approval.
	s.SetApprovalsForTest(map[engine.EngineID]engine.Approval{engine.EngineACMG2015: {ContentHash: "synthetic-annotation-test"}})
	id := accepted(t, s.Router(), annotationInput(), "")
	res := result(t, s.Router(), id)

	raw, err := json.Marshal(res.RuleEngineResult)
	if err != nil {
		t.Fatal(err)
	}
	var derived api.DerivedResult
	if err := json.Unmarshal(raw, &derived); err != nil {
		t.Fatalf("rule result shape: %v (%s)", err, string(raw))
	}
	if derived.Classification == "" {
		t.Fatalf("missing classification: %s", string(raw))
	}
	if derived.CombinationMethod != "svi_points" || derived.Points == nil {
		t.Fatalf("points method not applied: %+v", derived)
	}
	var codes []string
	for _, c := range derived.Derivation.Candidates {
		codes = append(codes, string(c.Code))
		if c.Code == "PM2" && c.Status == "met" && c.Strength != "Supporting" {
			t.Fatalf("PM2 strength %s", c.Strength)
		}
	}
	joined := strings.Join(codes, ",")
	for _, want := range []string{"PM2", "PP3", "PVS1"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing derived criterion %s in %s", want, joined)
		}
	}
	if derived.Points.PathogenicPts == 0 {
		t.Fatalf("points not accumulated: %+v", derived.Points)
	}
}

// evidenceFor reads the stored snapshot for a completed evaluation.
func evidenceFor(t *testing.T, s *api.Server, id string) evidence.EvidenceSnapshot {
	t.Helper()
	w := request(s.Router(), "GET", "/v1/evaluations/"+id+"/evidence", "", "test-api-key", "")
	if w.Code != 200 {
		t.Fatalf("snapshot unavailable: %d %s", w.Code, w.Body.String())
	}
	var snap evidence.EvidenceSnapshot
	if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	return snap
}

func TestCallerAnnotationIsNotSettled(t *testing.T) {
	s, closeServer := setupTestServer(t, false)
	defer closeServer()
	s.SetACMGEvaluator(acmg2015.NewEvaluator(nil))
	s.SetApprovalsForTest(map[engine.EngineID]engine.Approval{engine.EngineACMG2015: {ContentHash: "synthetic-annotation-test"}})
	id := accepted(t, s.Router(), annotationInput(), "")
	res := result(t, s.Router(), id)
	if res.ClinicalUse == "forbidden" {
		t.Fatal("local production result marked forbidden")
	}
	if !res.ReviewRequired {
		t.Fatal("caller annotation result must require review")
	}
	snap := evidenceFor(t, s, id)
	for _, src := range snap.Sources {
		if src.ValidationStatus == "verified" {
			t.Fatalf("caller-supplied source marked verified: %+v", src)
		}
	}
}

func TestAnnotationAssemblyMismatchRejected(t *testing.T) {
	s, closeServer := setupTestServer(t, true)
	defer closeServer()
	body := strings.Replace(annotationInput(), `"assembly":"GRCh38","status":"found","faf95"`, `"assembly":"GRCh37","status":"found","faf95"`, 1)
	if w := request(s.Router(), "POST", "/v1/variants/snv/evaluations", body, "test-api-key", ""); w.Code != 422 {
		t.Fatalf("assembly mismatch accepted: %d %s", w.Code, w.Body.String())
	}
}

func TestInvalidPhenotypeRejected(t *testing.T) {
	s, closeServer := setupTestServer(t, true)
	defer closeServer()
	body := strings.Replace(annotationInput(), `"annotations":`, `"phenotype":{"hpo_terms":[{"id":"NOT_HPO","polarity":"present"}]},"annotations":`, 1)
	if w := request(s.Router(), "POST", "/v1/variants/snv/evaluations", body, "test-api-key", ""); w.Code != 422 {
		t.Fatalf("invalid HPO accepted: %d", w.Code)
	}
	body = strings.Replace(annotationInput(), `"annotations":`, `"phenotype":{"hpo_terms":[{"id":"HP:0001945","polarity":"present"},{"id":"HP:0001945","polarity":"absent"}]},"annotations":`, 1)
	if w := request(s.Router(), "POST", "/v1/variants/snv/evaluations", body, "test-api-key", ""); w.Code != 422 {
		t.Fatalf("duplicate HPO accepted: %d", w.Code)
	}
}

func TestInvalidModeRejected(t *testing.T) {
	s, closeServer := setupTestServer(t, true)
	defer closeServer()
	body := strings.Replace(annotationInput(), `"mode":"local"`, `"mode":"auto"`, 1)
	if w := request(s.Router(), "POST", "/v1/variants/snv/evaluations", body, "test-api-key", ""); w.Code != 422 {
		t.Fatalf("invalid mode accepted: %d", w.Code)
	}
}

func TestAIAssistedGateWithoutApproval(t *testing.T) {
	s, closeServer := setupTestServer(t, false)
	defer closeServer()
	body := strings.Replace(annotationInput(), `"mode":"local"`, `"mode":"ai_assisted"`, 1)
	// The engine is still disabled in this fixture, so the approval gate wins.
	if w := request(s.Router(), "POST", "/v1/variants/snv/evaluations", body, "test-api-key", ""); w.Code != 409 {
		t.Fatalf("ai mode bypassed the approval gate: %d %s", w.Code, w.Body.String())
	}
}
