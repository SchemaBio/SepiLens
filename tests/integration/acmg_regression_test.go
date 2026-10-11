package integration_test

import (
	"encoding/json"
	"github.com/pzweuj/sepilens/internal/evidence"
	a "github.com/pzweuj/sepilens/internal/rules/acmg2015"
	"os"
	"testing"
)

func TestLegacyACMGFixturesAreNotVerifiedEvidence(t *testing.T) {
	b, e := os.ReadFile("../fixtures/acmg2015/golden_cases.json")
	if e != nil {
		t.Fatal(e)
	}
	var cases []struct {
		ID      string                 `json:"id"`
		Variant a.SNVVariantInput      `json:"variant"`
		Context a.ClinicalContextInput `json:"clinical_context"`
		Sources []evidence.Source      `json:"sources"`
	}
	if e = json.Unmarshal(b, &cases); e != nil {
		t.Fatal(e)
	}
	for _, c := range cases {
		t.Run(c.ID, func(t *testing.T) {
			out := a.NewEvaluator(nil).Evaluate(c.Variant, c.Context, c.Sources)
			if out.Classification != a.ClassVUS || out.IsVCEP || len(out.AppliedCriteria) != 0 {
				t.Fatalf("legacy unsupported inference: %+v", out)
			}
		})
	}
}
