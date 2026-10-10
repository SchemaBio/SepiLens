package cnv_test

import (
	"testing"

	"github.com/pzweuj/sepilens/internal/variant/cnv"
)

func TestAnnotator_KnownRegions(t *testing.T) {
	ann := cnv.NewAnnotator()

	// 1. Smith-Magenis full deletion (encompassing RAI1: chr17:16,000,000-21,000,000)
	smsInput := cnv.CNVInput{
		Assembly: "GRCh38",
		Chrom:    "chr17",
		Start:    16500000,
		End:      20500000,
		Type:     "loss",
	}
	resSMS := ann.Annotate(smsInput)
	if !resSMS.CompleteHIOverlap || resSMS.HighestHI != cnv.ScoreSufficient {
		t.Fatalf("expected complete HI overlap for Smith-Magenis region: %+v", resSMS)
	}

	// 2. 22q11.2 duplication (chr22:18,500,000-22,000,000)
	digeorgeInput := cnv.CNVInput{
		Assembly: "GRCh38",
		Chrom:    "22",
		Start:    18500000,
		End:      22000000,
		Type:     "gain",
	}
	resDG := ann.Annotate(digeorgeInput)
	if !resDG.CompleteTSOverlap || resDG.HighestTS != cnv.ScoreSufficient {
		t.Fatalf("expected complete TS overlap for 22q11.2 duplication: %+v", resDG)
	}

	// 3. Completely inside benign region
	benignInput := cnv.CNVInput{
		Assembly: "GRCh38",
		Chrom:    "1",
		Start:    72100000,
		End:      72400000,
		Type:     "loss",
	}
	resBenign := ann.Annotate(benignInput)
	if !resBenign.CompletelyInBenign || !resBenign.OverlapsBenign {
		t.Fatalf("expected completely in benign region: %+v", resBenign)
	}

	// 4. Gene desert / non-coding segment
	desertInput := cnv.CNVInput{
		Assembly: "GRCh38",
		Chrom:    "2",
		Start:    1000,
		End:      5000,
		Type:     "loss",
	}
	resDesert := ann.Annotate(desertInput)
	if resDesert.ContainsFunctionalElements || resDesert.GeneCount > 0 {
		t.Fatalf("expected no functional elements in desert: %+v", resDesert)
	}
}
