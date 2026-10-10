package integration_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pzweuj/sepilens/internal/rules/cnvgain2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvloss2020"
	"github.com/pzweuj/sepilens/internal/variant/cnv"
)

type CNVLossGoldenCase struct {
	ID                     string      `json:"id"`
	Description            string      `json:"description"`
	CNV                    cnv.CNVInput `json:"cnv"`
	CaseControlStatSig     bool        `json:"case_control_stat_sig"`
	CaseReportsCount       int         `json:"case_reports_count"`
	Inheritance            string      `json:"inheritance"`
	ExpectedScore          string      `json:"expected_score"`
	ExpectedClassification string      `json:"expected_classification"`
}

type CNVGainGoldenCase struct {
	ID                     string      `json:"id"`
	Description            string      `json:"description"`
	CNV                    cnv.CNVInput `json:"cnv"`
	TandemOrientationKnown bool        `json:"tandem_orientation_known"`
	CaseControlStatSig     bool        `json:"case_control_stat_sig"`
	CaseReportsCount       int         `json:"case_reports_count"`
	Inheritance            string      `json:"inheritance"`
	ExpectedScore          string      `json:"expected_score"`
	ExpectedClassification string      `json:"expected_classification"`
}

func TestCNV_Loss_GoldenCasesRegression(t *testing.T) {
	fixturePath := filepath.Join("..", "fixtures", "cnv_loss", "golden_cases.json")
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("failed to read loss golden cases: %v", err)
	}

	var cases []CNVLossGoldenCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("failed to parse loss golden cases: %v", err)
	}

	ann := cnv.NewAnnotator()
	engine := cnvloss2020.NewEngine()

	for _, tc := range cases {
		t.Run(tc.ID, func(t *testing.T) {
			annotated := ann.Annotate(tc.CNV)
			in := cnvloss2020.LossEvaluationInput{
				CNV:                annotated,
				CaseControlStatSig: tc.CaseControlStatSig,
				CaseReportsCount:   tc.CaseReportsCount,
				Inheritance:        tc.Inheritance,
			}
			res := engine.Evaluate(in)

			if res.Classification != tc.ExpectedClassification {
				t.Fatalf("[%s]: expected classification %s, got %s", tc.ID, tc.ExpectedClassification, res.Classification)
			}
			if res.TotalScore != tc.ExpectedScore {
				t.Fatalf("[%s]: expected score %s, got %s", tc.ID, tc.ExpectedScore, res.TotalScore)
			}
		})
	}
}

func TestCNV_Gain_GoldenCasesRegression(t *testing.T) {
	fixturePath := filepath.Join("..", "fixtures", "cnv_gain", "golden_cases.json")
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("failed to read gain golden cases: %v", err)
	}

	var cases []CNVGainGoldenCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("failed to parse gain golden cases: %v", err)
	}

	ann := cnv.NewAnnotator()
	engine := cnvgain2020.NewEngine()

	for _, tc := range cases {
		t.Run(tc.ID, func(t *testing.T) {
			annotated := ann.Annotate(tc.CNV)
			in := cnvgain2020.GainEvaluationInput{
				CNV:                    annotated,
				TandemOrientationKnown: tc.TandemOrientationKnown,
				CaseControlStatSig:     tc.CaseControlStatSig,
				CaseReportsCount:       tc.CaseReportsCount,
				Inheritance:            tc.Inheritance,
			}
			res := engine.Evaluate(in)

			if res.Classification != tc.ExpectedClassification {
				t.Fatalf("[%s]: expected classification %s, got %s", tc.ID, tc.ExpectedClassification, res.Classification)
			}
			if res.TotalScore != tc.ExpectedScore {
				t.Fatalf("[%s]: expected score %s, got %s", tc.ID, tc.ExpectedScore, res.TotalScore)
			}
		})
	}
}
