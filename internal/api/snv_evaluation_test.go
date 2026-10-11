package api_test

import (
	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
	"testing"
)

func TestInjectedSNVEngineCannotBypassApproval(t *testing.T) {
	s, close := setupTestServer(t, false)
	defer close()
	s.SetACMGEvaluator(acmg2015.NewEvaluator(nil))
	w := request(s.Router(), "POST", "/v1/variants/snv/evaluations", snvInput, "test-api-key", "")
	if w.Code != 409 {
		t.Fatal("injection bypassed gate", w.Code)
	}
}
