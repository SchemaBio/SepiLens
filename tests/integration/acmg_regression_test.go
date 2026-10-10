package integration_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
)

type ACMGGoldenCase struct {
	ID                     string                     `json:"id"`
	Description            string                     `json:"description"`
	Variant                acmg2015.SNVVariantInput   `json:"variant"`
	ClinicalContext        acmg2015.ClinicalContextInput `json:"clinical_context"`
	Sources                []evidence.Source          `json:"sources"`
	ExpectedClassification string                     `json:"expected_classification"`
	ExpectedVCEP           bool                       `json:"expected_vcep"`
	ExpectConflict         bool                       `json:"expect_conflict"`
}

func TestACMG_GoldenCasesRegression(t *testing.T) {
	fixturePath := filepath.Join("..", "fixtures", "acmg2015", "golden_cases.json")
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("failed to read ACMG golden cases fixture: %v", err)
	}

	var cases []ACMGGoldenCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("failed to parse ACMG golden cases fixture: %v", err)
	}

	eval := acmg2015.NewEvaluator(nil)

	for _, tc := range cases {
		t.Run(tc.ID, func(t *testing.T) {
			out := eval.Evaluate(tc.Variant, tc.ClinicalContext, tc.Sources)

			if out.Classification != tc.ExpectedClassification {
				t.Fatalf("REGRESSION FAILURE [%s]: expected classification %s, got %s (out: %+v)",
					tc.ID, tc.ExpectedClassification, out.Classification, out)
			}

			if out.IsVCEP != tc.ExpectedVCEP {
				t.Fatalf("[%s]: expected IsVCEP=%v, got %v", tc.ID, tc.ExpectedVCEP, out.IsVCEP)
			}

			if tc.ExpectConflict && len(out.RuleConflicts) == 0 {
				t.Fatalf("[%s]: expected conflict to be recorded", tc.ID)
			}
		})
	}
}
