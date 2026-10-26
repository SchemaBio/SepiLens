package api

import (
	"github.com/pzweuj/sepilens/internal/assessment"
	"github.com/pzweuj/sepilens/internal/llm"
	"strings"
	"testing"
)

func TestSVCOutputContractRejectsCrossEngineAndInventedMethodFields(t *testing.T) {
	schema := assessmentSchemaFor(assessment.Request{EngineID: "svc_v4"})
	valid := `{"criteria":[{"code":"PFD","status":"insufficient_data","strength":"","score_hundredths":null,"points":null,"source_ids":[],"facts_confirmed":[],"change":"confirmed","override_reason":"unknown"}],"classification":"VUS"}`
	if err := llm.ValidateJSON(valid, schema); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{strings.Replace(valid, `"PFD"`, `"PS3"`, 1), strings.Replace(valid, `"strength":""`, `"strength":"Strong"`, 1), strings.Replace(valid, `"points":null`, `"points":3`, 1), strings.Replace(valid, `"facts_confirmed":[]`, `"facts_confirmed":["confirmed_de_novo"]`, 1), strings.Replace(valid, `"score_hundredths":null`, `"score_hundredths":100`, 1)} {
		if llm.ValidateJSON(raw, schema) == nil {
			t.Fatal("unsafe model structure accepted")
		}
	}
}
