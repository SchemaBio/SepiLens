package acmg2015

import (
	"fmt"
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
	Consequence     string   `json:"consequence,omitempty"`
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
	Classification     string             `json:"classification"`
	CombinationPath    string             `json:"combination_path"`
	AppliedCriteria    []AppliedCriterion `json:"applied_criteria"`
	RuleConflicts      []string           `json:"rule_conflicts,omitempty"`
	SpecificationID    string             `json:"specification_id"`
	IsVCEP             bool               `json:"is_vcep"`
	ReviewRequired     bool               `json:"review_required"`
	MissingInformation []string           `json:"missing_information,omitempty"`
}
type Evaluator struct {
	registry *SpecificationRegistry
	combiner *Combiner
}

func NewEvaluator(r *SpecificationRegistry) *Evaluator {
	if r == nil {
		r = NewSpecificationRegistry()
	}
	return &Evaluator{r, NewCombiner()}
}

// Evaluate never infers clinical evidence from a consequence, an absent annotation or prose keywords.
func (e *Evaluator) Evaluate(v SNVVariantInput, c ClinicalContextInput, s []evidence.Source) EvaluationOutput {
	return e.EvaluateEvidence(v, c, nil, s)
}
func (e *Evaluator) EvaluateEvidence(v SNVVariantInput, c ClinicalContextInput, criteria []AppliedCriterion, sources []evidence.Source) EvaluationOutput {
	spec := e.registry.Resolve(v.Gene, c.DiseaseID, c.Inheritance)
	out := EvaluationOutput{Classification: ClassVUS, SpecificationID: spec.ID, IsVCEP: spec.IsVCEP, ReviewRequired: true}
	verified := map[string]bool{}
	for _, s := range sources {
		verified[s.ID] = s.ValidationStatus == evidence.ValidationVerified && s.IdentifierValid && s.RecordExists && s.ObjectMatched && s.MaterialVerified
	}
	for _, crit := range criteria {
		def, exists := CanonicalCriteria[crit.Code]
		valid := exists && def.Direction == crit.Direction && len(crit.SourceIDs) > 0
		// Baseline strengths cannot be upgraded by a model or caller.
		valid = valid && AllowedStrength(crit.Code, crit.EvaluatedStrength)
		for _, fact := range RequiredFacts[crit.Code] {
			valid = valid && crit.Facts[fact]
		}
		switch crit.Code {
		case CodePM2, CodeBA1, CodeBS1:
			valid = valid && v.AlleleFrequency != nil
			if v.AlleleFrequency != nil {
				valid = valid && *v.AlleleFrequency >= 0 && *v.AlleleFrequency <= 1
				if crit.Code == CodeBA1 {
					valid = valid && *v.AlleleFrequency > .05
				}
				if crit.Code == CodePM2 {
					valid = valid && *v.AlleleFrequency < .05
				}
			}
		}
		switch crit.EvaluatedStrength {
		case StrengthStandAlone:
			valid = valid && crit.Code == CodeBA1
		case StrengthVeryStrong:
			valid = valid && crit.Direction == DirectionPathogenic
		case StrengthStrong, StrengthModerate, StrengthSupporting:
		default:
			valid = false
		}
		for _, id := range crit.SourceIDs {
			valid = valid && verified[id]
		}
		for _, other := range criteria {
			if other.Code == crit.Code || other.Status != CriterionStatusMet {
				continue
			}
			for _, id := range crit.SourceIDs {
				for _, oid := range other.SourceIDs {
					if id == oid && !crit.Facts["evidence_independent"] {
						valid = false
					}
				}
			}
		}
		if !valid {
			out.MissingInformation = append(out.MissingInformation, fmt.Sprintf("%s requires verified structured supporting material", crit.Code))
			continue
		}
		out.AppliedCriteria = append(out.AppliedCriteria, crit)
	}
	if len(out.AppliedCriteria) == 0 {
		out.MissingInformation = append(out.MissingInformation, "no verified criterion material supplied")
	}
	res := e.combiner.Combine(out.AppliedCriteria)
	out.Classification = res.Classification
	out.CombinationPath = res.PathogenicPath
	out.RuleConflicts = res.Conflicts
	return out
}
