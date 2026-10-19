package api_test

import (
	"strings"
	"testing"
)

func TestAnnotationValidateEndpoint(t *testing.T) {
	s, closeServer := setupTestServer(t, false)
	defer closeServer()
	body := `{"engine_id":"acmg_amp_2015","variant":{"assembly":"GRCh38","chrom":"7","pos":100,"ref":"A","alt":"G","gene":"CFTR"},"annotations":{"source":"caller_supplied","gnomad":{"version":"v4.1","assembly":"GRCh38","status":"found","faf95":0.00001,"allele_number":1200000,"coverage_adequate":true}}}`
	w := request(s.Router(), "POST", "/v1/annotations/validate", body, "test-api-key", "")
	if w.Code != 200 {
		t.Fatalf("validate: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"complete":true`) {
		t.Fatalf("complete flag missing: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "not verified evidence") {
		t.Fatalf("settlement warning missing: %s", w.Body.String())
	}
}

func TestAnnotationValidateRejectsBadPayload(t *testing.T) {
	s, closeServer := setupTestServer(t, false)
	defer closeServer()
	cases := []string{
		`{"engine_id":"acmg_amp_2015","variant":{"assembly":"GRCh38"},"annotations":{}}`,
		`{"engine_id":"acmg_amp_2015","variant":{"assembly":"GRCh38"},"annotations":{"gnomad":{"assembly":"GRCh37","version":"v4","status":"found"}}}`,
		`{"engine_id":"acmg_amp_2015","variant":{"assembly":"GRCh38"},"annotations":{"gnomad":{"version":"","status":"found"}}}`,
	}
	for _, body := range cases {
		if w := request(s.Router(), "POST", "/v1/annotations/validate", body, "test-api-key", ""); w.Code != 422 {
			t.Fatalf("accepted invalid payload: %d %s", w.Code, body)
		}
	}
}

func TestAnnotationSourcesEndpoint(t *testing.T) {
	s, closeServer := setupTestServer(t, false)
	defer closeServer()
	w := request(s.Router(), "GET", "/v1/annotations/sources", "", "test-api-key", "")
	if w.Code != 200 {
		t.Fatalf("sources: %d", w.Code)
	}
	for _, id := range []string{"clinvar", "gnomad", "alphamissense", "pangolin", "gencc"} {
		if !strings.Contains(w.Body.String(), `"id":"`+id+`"`) && !strings.Contains(w.Body.String(), `"id": "`+id+`"`) {
			t.Fatalf("missing source %s: %s", id, w.Body.String())
		}
	}
}
