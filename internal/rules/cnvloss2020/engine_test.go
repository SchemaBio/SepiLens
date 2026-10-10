package cnvloss2020_test

import (
	"testing"

	"github.com/pzweuj/sepilens/internal/rules/cnvloss2020"
	"github.com/pzweuj/sepilens/internal/variant/cnv"
)

func TestLossEngine_Cases(t *testing.T) {
	engine := cnvloss2020.NewEngine()

	// 1. Smith-Magenis deletion (Complete HI overlap: RAI1, score = +1.00 -> Pathogenic)
	inHI := cnvloss2020.LossEvaluationInput{
		CNV: cnv.AnnotatedCNV{
			ContainsFunctionalElements: true,
			CompleteHIOverlap:          true,
			HighestHIGene:              "RAI1",
			GeneCount:                  5,
		},
	}
	resHI := engine.Evaluate(inHI)
	if resHI.Classification != "Pathogenic" || resHI.TotalScoreHundredths != 100 {
		t.Fatalf("expected Pathogenic (100 hundredths), got %s (%d): %+v", resHI.Classification, resHI.TotalScoreHundredths, resHI)
	}

	// 2. Gene desert (Sec 1B: -0.60 -> VUS)
	inDesert := cnvloss2020.LossEvaluationInput{
		CNV: cnv.AnnotatedCNV{
			ContainsFunctionalElements: false,
			GeneCount:                  0,
		},
	}
	resDesert := engine.Evaluate(inDesert)
	if resDesert.Classification != "VUS" || resDesert.TotalScoreHundredths != -60 {
		t.Fatalf("expected VUS (-60 hundredths), got %s (%d)", resDesert.Classification, resDesert.TotalScoreHundredths)
	}

	// 3. Completely in benign region (Sec 2F: -1.00 -> Benign)
	inBenign := cnvloss2020.LossEvaluationInput{
		CNV: cnv.AnnotatedCNV{
			ContainsFunctionalElements: true,
			CompletelyInBenign:         true,
			GeneCount:                  2,
		},
	}
	resBenign := engine.Evaluate(inBenign)
	if resBenign.Classification != "Benign" || resBenign.TotalScoreHundredths != -100 {
		t.Fatalf("expected Benign (-100 hundredths), got %s (%d)", resBenign.Classification, resBenign.TotalScoreHundredths)
	}

	// 4. Gene-rich deletion: 30 genes (+0.45) + De novo confirmed (+0.45) = +0.90 -> Likely pathogenic
	inGeneRich := cnvloss2020.LossEvaluationInput{
		CNV: cnv.AnnotatedCNV{
			ContainsFunctionalElements: true,
			GeneCount:                  30,
		},
		Inheritance: "de_novo_confirmed",
	}
	resRich := engine.Evaluate(inGeneRich)
	if resRich.Classification != "Likely pathogenic" || resRich.TotalScoreHundredths != 90 {
		t.Fatalf("expected Likely pathogenic (90 hundredths), got %s (%d)", resRich.Classification, resRich.TotalScoreHundredths)
	}
}
