package acmg2015_test

import (
	"testing"

	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
)

func TestEvaluator_CFTR_Nonsense_Pathogenic(t *testing.T) {
	reg := acmg2015.NewSpecificationRegistry()
	eval := acmg2015.NewEvaluator(reg)

	zeroAF := 0.00001
	variant := acmg2015.SNVVariantInput{
		Assembly:        "GRCh38",
		Chrom:           "chr7",
		Pos:             117548628,
		Ref:             "G",
		Alt:             "T",
		Transcript:      "NM_000492.3",
		HGVSc:           "c.1624G>T",
		HGVSp:           "p.Gly542Ter",
		Gene:            "CFTR",
		Consequence:     "nonsense",
		AlleleFrequency: &zeroAF,
	}

	clinicalCtx := acmg2015.ClinicalContextInput{
		DiseaseID:     "MONDO:0009061",
		Inheritance:   "AR",
		VariantOrigin: "germline_confirmed",
	}

	sources := []evidence.Source{
		{
			ID:               "SRC-PMID-FUNC",
			Type:             evidence.SourceTypePubMed,
			PMID:             "12345678",
			Title:            "Functional analysis of CFTR nonsense mutation",
			Snippet:          "Patch clamp showed pathogenic functional effect with loss of chloride transport.",
			ValidationStatus: evidence.ValidationVerified,
		},
	}

	out := eval.Evaluate(variant, clinicalCtx, sources)
	if out.Classification != acmg2015.ClassPathogenic {
		t.Fatalf("expected Pathogenic for CFTR G542X with functional evidence, got %s (out: %+v)", out.Classification, out)
	}
	if !out.IsVCEP || out.SpecificationID != "clingen_cftr_vcep_v1" {
		t.Fatalf("expected CFTR VCEP to be applied, got spec %s", out.SpecificationID)
	}
}

func TestEvaluator_HighFrequency_Benign(t *testing.T) {
	eval := acmg2015.NewEvaluator(nil)

	highAF := 0.08 // 8% population frequency -> BA1
	variant := acmg2015.SNVVariantInput{
		Assembly:        "GRCh38",
		Chrom:           "chr1",
		Pos:             12345678,
		Ref:             "A",
		Alt:             "G",
		Gene:            "GENE1",
		Consequence:     "missense",
		AlleleFrequency: &highAF,
	}

	clinicalCtx := acmg2015.ClinicalContextInput{
		DiseaseID:   "MONDO:0000001",
		Inheritance: "AD",
	}

	out := eval.Evaluate(variant, clinicalCtx, nil)
	if out.Classification != acmg2015.ClassBenign {
		t.Fatalf("expected Benign due to BA1, got %s", out.Classification)
	}
}

func TestEvaluator_ConflictingCriteria_VUS(t *testing.T) {
	eval := acmg2015.NewEvaluator(nil)

	// Nonsense mutation (PVS1) but high population frequency (BA1)
	conflictAF := 0.06
	variant := acmg2015.SNVVariantInput{
		Assembly:        "GRCh38",
		Chrom:           "chr7",
		Pos:             117548628,
		Ref:             "G",
		Alt:             "T",
		HGVSp:           "p.Gly542Ter",
		Gene:            "CFTR",
		Consequence:     "nonsense",
		AlleleFrequency: &conflictAF,
	}

	clinicalCtx := acmg2015.ClinicalContextInput{
		DiseaseID:   "MONDO:0009061",
		Inheritance: "AR",
	}

	sources := []evidence.Source{
		{
			ID:               "SRC-1",
			Snippet:          "Damaging functional assay",
			ValidationStatus: evidence.ValidationVerified,
		},
	}

	out := eval.Evaluate(variant, clinicalCtx, sources)
	if out.Classification != acmg2015.ClassVUS {
		t.Fatalf("expected VUS due to conflicting pathogenic and benign criteria, got %s", out.Classification)
	}
	if len(out.RuleConflicts) == 0 {
		t.Fatalf("expected rule conflicts to be recorded")
	}
}
