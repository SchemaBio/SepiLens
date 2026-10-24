package llm

import "testing"

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
