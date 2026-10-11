package api_test

import (
	"strings"
	"testing"
)

func TestUnknownOriginEvidenceOnlyFreeRead(t *testing.T) {
	s, close := setupTestServer(t, false)
	defer close()
	h := s.Router()
	id := accepted(t, h, strings.Replace(snvInput, "germline_confirmed", "unknown", 1), "")
	v := result(t, h, id)
	if v.ResultKind != "evidence_only" || v.SettlementEligible || v.RuleEngineResult != nil {
		t.Fatal("unknown origin classified or billed")
	}
	again := result(t, h, id)
	if again.EvaluationID != id || again.SettlementEligible {
		t.Fatal("historical read changed billing")
	}
}
