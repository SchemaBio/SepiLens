package api_test

import (
	"github.com/pzweuj/sepilens/internal/rules/cnvgain2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvloss2020"
	"github.com/pzweuj/sepilens/internal/variant/cnv"
	"strings"
	"testing"
)

func TestCNVGateAndCoordinates(t *testing.T) {
	s, close := setupTestServer(t, false)
	defer close()
	s.SetCNVEngines(cnvloss2020.NewEngine(), cnvgain2020.NewEngine(), cnv.NewAnnotator())
	if w := request(s.Router(), "POST", "/v1/variants/cnv/evaluations", cnvInput, "test-api-key", ""); w.Code != 409 {
		t.Fatal("gate bypass", w.Code)
	}
	for _, body := range []string{strings.Replace(cnvInput, "1-based-inclusive", "invalid", 1), strings.Replace(cnvInput, `"start":100`, `"start":0`, 1), strings.Replace(cnvInput, "germline_confirmed", "somatic", 1)} {
		if w := request(s.Router(), "POST", "/v1/variants/cnv/evaluations", body, "test-api-key", ""); w.Code != 422 {
			t.Fatal("invalid CNV accepted", w.Code)
		}
	}
}
