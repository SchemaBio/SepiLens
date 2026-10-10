package cnvgain2020_test

import (
	"testing"

	"github.com/pzweuj/sepilens/internal/rules/cnvgain2020"
	"github.com/pzweuj/sepilens/internal/variant/cnv"
)

func TestGainEngine_Cases(t *testing.T) {
	engine := cnvgain2020.NewEngine()

	// 1. 22q11.2 complete TS overlap (score = +1.00 -> Pathogenic)
	inTS := cnvgain2020.GainEvaluationInput{
		CNV: cnv.AnnotatedCNV{
			ContainsFunctionalElements: true,
			CompleteTSOverlap:          true,
			HighestTSGene:              "22q11.2_region",
			GeneCount:                  30,
		},
	}
	resTS := engine.Evaluate(inTS)
	if resTS.Classification != "Pathogenic" || resTS.TotalScoreHundredths != 100 {
		t.Fatalf("expected Pathogenic (100 hundredths), got %s (%d)", resTS.Classification, resTS.TotalScoreHundredths)
	}

	// 2. 35 genes (Gain Sec 3B: +45) -> VUS
	in35Genes := cnvgain2020.GainEvaluationInput{
		CNV: cnv.AnnotatedCNV{
			ContainsFunctionalElements: true,
			GeneCount:                  35, // For Gain: 35-49 is +0.45
		},
	}
	res35 := engine.Evaluate(in35Genes)
	if res35.Classification != "VUS" || res35.TotalScoreHundredths != 45 {
		t.Fatalf("expected VUS (45 hundredths), got %s (%d)", res35.Classification, res35.TotalScoreHundredths)
	}

	// 3. 50 genes (Gain Sec 3C: +90) + De novo confirmed (+45) = +135 -> Pathogenic
	in50Genes := cnvgain2020.GainEvaluationInput{
		CNV: cnv.AnnotatedCNV{
			ContainsFunctionalElements: true,
			GeneCount:                  52, // >= 50 is +0.90
		},
		Inheritance: "de_novo_confirmed",
	}
	res50 := engine.Evaluate(in50Genes)
	if res50.Classification != "Pathogenic" || res50.TotalScoreHundredths != 135 {
		t.Fatalf("expected Pathogenic (135 hundredths), got %s (%d)", res50.Classification, res50.TotalScoreHundredths)
	}

	// 4. Completely in benign gain region (-1.00 -> Benign)
	inBenign := cnvgain2020.GainEvaluationInput{
		CNV: cnv.AnnotatedCNV{
			ContainsFunctionalElements: true,
			CompletelyInBenign:         true,
			GeneCount:                  1,
		},
	}
	resBenign := engine.Evaluate(inBenign)
	if resBenign.Classification != "Benign" || resBenign.TotalScoreHundredths != -100 {
		t.Fatalf("expected Benign (-100 hundredths), got %s (%d)", resBenign.Classification, resBenign.TotalScoreHundredths)
	}
}
