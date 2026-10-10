package acmg2015

import (
	"fmt"
	"strings"

	"github.com/pzweuj/sepilens/internal/evidence"
)

type SNVVariantInput struct {
	Assembly        string   `json:"assembly"`
	Chrom           string   `json:"chrom"`
	Pos             int64    `json:"pos"`
	Ref             string   `json:"ref"`
	Alt             string   `json:"alt"`
	Transcript      string   `json:"transcript,omitempty"`
	HGVSc           string   `json:"hgvs_c,omitempty"`
	HGVSp           string   `json:"hgvs_p,omitempty"`
	Gene            string   `json:"gene"`
	Consequence     string   `json:"consequence,omitempty"` // e.g. "nonsense", "frameshift", "missense", "synonymous"
	AlleleFrequency *float64 `json:"allele_frequency,omitempty"`
}

type ClinicalContextInput struct {
	DiseaseID     string   `json:"disease_id"`
	Inheritance   string   `json:"inheritance"`
	VariantOrigin string   `json:"variant_origin"`
	SampleType    string   `json:"sample_type"`
	HPOTerms      []string `json:"hpo_terms,omitempty"`
}

type EvaluationOutput struct {
	Classification  string             `json:"classification"`
	CombinationPath string             `json:"combination_path"`
	AppliedCriteria []AppliedCriterion `json:"applied_criteria"`
	RuleConflicts   []string           `json:"rule_conflicts,omitempty"`
	SpecificationID string             `json:"specification_id"`
	IsVCEP          bool               `json:"is_vcep"`
	ReviewRequired  bool               `json:"review_required"`
}

type Evaluator struct {
	registry *SpecificationRegistry
	combiner *Combiner
}

func NewEvaluator(registry *SpecificationRegistry) *Evaluator {
	if registry == nil {
		registry = NewSpecificationRegistry()
	}
	return &Evaluator{
		registry: registry,
		combiner: NewCombiner(),
	}
}

func (e *Evaluator) Evaluate(variant SNVVariantInput, ctx ClinicalContextInput, sources []evidence.Source) EvaluationOutput {
	spec := e.registry.Resolve(variant.Gene, ctx.DiseaseID, ctx.Inheritance)

	var applied []AppliedCriterion

	// 1. Population allele frequency criteria: BA1, BS1, PM2
	if variant.AlleleFrequency != nil {
		af := *variant.AlleleFrequency
		if af >= spec.BA1Threshold {
			applied = append(applied, AppliedCriterion{
				Code:              CodeBA1,
				EvaluatedStrength: StrengthStandAlone,
				Direction:         DirectionBenign,
				Status:            CriterionStatusMet,
				Rationale:         fmt.Sprintf("Allele frequency (%.4f) >= BA1 threshold (%.4f)", af, spec.BA1Threshold),
			})
		} else if af >= spec.BS1Threshold {
			applied = append(applied, AppliedCriterion{
				Code:              CodeBS1,
				EvaluatedStrength: StrengthStrong,
				Direction:         DirectionBenign,
				Status:            CriterionStatusMet,
				Rationale:         fmt.Sprintf("Allele frequency (%.4f) >= BS1 threshold (%.4f)", af, spec.BS1Threshold),
			})
		}

		if af <= spec.PM2Threshold {
			applied = append(applied, AppliedCriterion{
				Code:              CodePM2,
				EvaluatedStrength: spec.PM2Strength,
				Direction:         DirectionPathogenic,
				Status:            CriterionStatusMet,
				Rationale:         fmt.Sprintf("Allele frequency (%.6f) <= PM2 threshold (%.6f)", af, spec.PM2Threshold),
			})
		}
	} else {
		// If population frequency is absent, apply PM2 at specified strength as default assumption for rare variant
		applied = append(applied, AppliedCriterion{
			Code:              CodePM2,
			EvaluatedStrength: spec.PM2Strength,
			Direction:         DirectionPathogenic,
			Status:            CriterionStatusMet,
			Rationale:         "Variant is absent from population control databases",
		})
	}

	// 2. Consequence / Molecular mechanism criteria: PVS1, BP7, PM4
	csq := strings.ToLower(variant.Consequence)
	hgvsP := variant.HGVSp
	hgvsC := strings.ToLower(variant.HGVSc)

	if spec.PVS1Applicable && isNullVariant(csq, hgvsP, hgvsC) {
		applied = append(applied, AppliedCriterion{
			Code:              CodePVS1,
			EvaluatedStrength: StrengthVeryStrong,
			Direction:         DirectionPathogenic,
			Status:            CriterionStatusMet,
			Rationale:         fmt.Sprintf("Null variant in %s where loss of function is established mechanism", variant.Gene),
		})
	}

	if csq == "synonymous" {
		applied = append(applied, AppliedCriterion{
			Code:              CodeBP7,
			EvaluatedStrength: StrengthSupporting,
			Direction:         DirectionBenign,
			Status:            CriterionStatusMet,
			Rationale:         "Synonymous variant with no predicted splice site alteration",
		})
	}

	// 3. Integrate retrieval sources (functional assay PS3/BS3, hotspot PM1, ClinVar reports)
	for _, src := range sources {
		if src.ValidationStatus != evidence.ValidationVerified {
			continue
		}

		snippetLower := strings.ToLower(src.Snippet + " " + src.Title)
		if strings.Contains(snippetLower, "damaging") || strings.Contains(snippetLower, "pathogenic functional effect") {
			applied = append(applied, AppliedCriterion{
				Code:              CodePS3,
				EvaluatedStrength: StrengthStrong,
				Direction:         DirectionPathogenic,
				Status:            CriterionStatusMet,
				Rationale:         fmt.Sprintf("Functional study supportive of damaging effect from %s", src.ID),
				SourceIDs:         []string{src.ID},
			})
		}

		if strings.Contains(snippetLower, "no damaging effect") || strings.Contains(snippetLower, "normal function") {
			applied = append(applied, AppliedCriterion{
				Code:              CodeBS3,
				EvaluatedStrength: StrengthStrong,
				Direction:         DirectionBenign,
				Status:            CriterionStatusMet,
				Rationale:         fmt.Sprintf("Functional study shows normal activity from %s", src.ID),
				SourceIDs:         []string{src.ID},
			})
		}

		if strings.Contains(snippetLower, "hotspot") || strings.Contains(snippetLower, "critical domain") {
			applied = append(applied, AppliedCriterion{
				Code:              CodePM1,
				EvaluatedStrength: StrengthModerate,
				Direction:         DirectionPathogenic,
				Status:            CriterionStatusMet,
				Rationale:         fmt.Sprintf("Located in critical functional domain from %s", src.ID),
				SourceIDs:         []string{src.ID},
			})
		}
	}

	// 4. Combine criteria deterministically
	combRes := e.combiner.Combine(applied)

	return EvaluationOutput{
		Classification:  combRes.Classification,
		CombinationPath: combRes.PathogenicPath,
		AppliedCriteria: applied,
		RuleConflicts:   combRes.Conflicts,
		SpecificationID: spec.ID,
		IsVCEP:          spec.IsVCEP,
		ReviewRequired:  true,
	}
}

func isNullVariant(consequence, hgvsP, hgvsC string) bool {
	if consequence == "nonsense" || consequence == "frameshift" ||
		consequence == "splice_donor" || consequence == "splice_acceptor" {
		return true
	}
	if strings.Contains(hgvsP, "Ter") || strings.Contains(hgvsP, "*") || strings.Contains(hgvsP, "fs") {
		return true
	}
	if strings.Contains(hgvsC, "del") && strings.Contains(consequence, "shift") {
		return true
	}
	return false
}
