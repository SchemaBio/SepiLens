package cnvgain2020_test

import (
	"github.com/pzweuj/sepilens/internal/rules/cnvgain2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvrubric"
	"github.com/pzweuj/sepilens/internal/variant/cnv"
	"testing"
)

func TestGainRequiresTSInsteadOfHI(t *testing.T) {
	e := cnvgain2020.NewEngine()
	if e.Evaluate(cnvgain2020.GainEvaluationInput{}).Classification != "insufficient_data" {
		t.Fatal("missing dataset scored")
	}
	in := cnvgain2020.GainEvaluationInput{CNV: cnv.AnnotatedCNV{CoverageComplete: true, CompleteHIOverlap: true, CompleteTSOverlap: true}, Evidence: []cnvrubric.Selection{{Code: "2A", Score: 100, SourceIDs: []string{"source"}, Verified: true, Facts: map[string]bool{"complete_hi_overlap": true}}}}
	if e.Evaluate(in).Classification != "insufficient_data" {
		t.Fatal("HI alone supports duplication")
	}
	in.Evidence[0].Facts = map[string]bool{"complete_ts_overlap": true}
	r := e.Evaluate(in)
	if r.TotalScoreHundredths != 100 || r.Classification != "Pathogenic" {
		t.Fatalf("official gain 2A: %+v", r)
	}
}
