package llm

import (
	"strings"
	"testing"
)

func TestNullableScoresRemainStrictlyValidated(t *testing.T) {
	for _, values := range []any{[]any{"integer", "null"}, []string{"integer", "null"}} {
		for _, test := range []struct {
			raw   string
			valid bool
		}{{"null", true}, {"2", true}, {"2.5", false}, {`"2"`, false}, {"true", false}, {"{}", false}} {
			err := ValidateJSON(test.raw, map[string]any{"type": values})
			if (err == nil) != test.valid {
				t.Errorf("%s: %v", test.raw, err)
			}
		}
	}
}

func TestSchemaPathsAndNativeIntegerBounds(t *testing.T) {
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": []any{"criteria"}, "properties": map[string]any{"criteria": map[string]any{"type": "array", "maxItems": 1, "uniqueItems": true, "items": map[string]any{"type": "object", "properties": map[string]any{"points": map[string]any{"type": "null"}}}}}}
	for _, raw := range []string{`{"criteria":[{"points":3}]}`, `{"criteria":[{},{}]}`, `{"private-patient-key":"private-patient-value"}`} {
		err := ValidateJSON(raw, schema)
		if err == nil {
			t.Fatal("invalid structure accepted")
		}
		if strings.Contains(err.Error(), "private-patient") {
			t.Fatal("model values leaked in diagnostic")
		}
	}
	err := ValidateJSON(`{"criteria":[{"points":3}]}`, schema)
	if err.Error() != "$.criteria[0].points: null required" {
		t.Fatal(err)
	}
	if ValidateJSON(`"too long"`, map[string]any{"type": "string", "maxLength": 3}) == nil {
		t.Fatal("integer maxLength ignored")
	}
	if ValidateJSON(`["POP","POP"]`, map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string"}}) == nil {
		t.Fatal("duplicate facts accepted")
	}
}
