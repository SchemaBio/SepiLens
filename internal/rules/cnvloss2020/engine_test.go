package cnvloss2020_test

import (
	"github.com/pzweuj/sepilens/internal/rules/cnvloss2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvrubric"
	"github.com/pzweuj/sepilens/internal/variant/cnv"
	"testing"
)

func TestLossRequiresMaterial(t *testing.T) {
	e := cnvloss2020.NewEngine()
	if e.Evaluate(cnvloss2020.LossEvaluationInput{}).Classification != "insufficient_data" {
		t.Fatal("missing dataset scored")
	}
	in := cnvloss2020.LossEvaluationInput{CNV: cnv.AnnotatedCNV{CoverageComplete: true, CompleteHIOverlap: true, CompleteTSOverlap: true}, Evidence: []cnvrubric.Selection{{Code: "2A", Score: 100, SourceIDs: []string{"source"}, Verified: true, Facts: map[string]bool{"complete_hi_overlap": true}}}}
	r := e.Evaluate(in)
	if r.TotalScoreHundredths != 100 || r.Classification != "Pathogenic" {
		t.Fatalf("official loss 2A: %+v", r)
	}
	in.Evidence[0].Facts = nil
	if e.Evaluate(in).Classification != "insufficient_data" {
		t.Fatal("missing HI material scored")
	}
}
