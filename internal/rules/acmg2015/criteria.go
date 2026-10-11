package acmg2015

import (
	"errors"
	"fmt"
	"strings"
)

type CriterionCode string

const (
	// Pathogenic criteria
	CodePVS1 CriterionCode = "PVS1"
	CodePS1  CriterionCode = "PS1"
	CodePS2  CriterionCode = "PS2"
	CodePS3  CriterionCode = "PS3"
	CodePS4  CriterionCode = "PS4"
	CodePM1  CriterionCode = "PM1"
	CodePM2  CriterionCode = "PM2"
	CodePM3  CriterionCode = "PM3"
	CodePM4  CriterionCode = "PM4"
	CodePM5  CriterionCode = "PM5"
	CodePM6  CriterionCode = "PM6"
	CodePP1  CriterionCode = "PP1"
	CodePP2  CriterionCode = "PP2"
	CodePP3  CriterionCode = "PP3"
	CodePP4  CriterionCode = "PP4"
	CodePP5  CriterionCode = "PP5"

	// Benign criteria
	CodeBA1 CriterionCode = "BA1"
	CodeBS1 CriterionCode = "BS1"
	CodeBS2 CriterionCode = "BS2"
	CodeBS3 CriterionCode = "BS3"
	CodeBS4 CriterionCode = "BS4"
	CodeBP1 CriterionCode = "BP1"
	CodeBP2 CriterionCode = "BP2"
	CodeBP3 CriterionCode = "BP3"
	CodeBP4 CriterionCode = "BP4"
	CodeBP5 CriterionCode = "BP5"
	CodeBP6 CriterionCode = "BP6"
	CodeBP7 CriterionCode = "BP7"
)

type Strength string

const (
	StrengthStandAlone Strength = "StandAlone"
	StrengthVeryStrong Strength = "VeryStrong"
	StrengthStrong     Strength = "Strong"
	StrengthModerate   Strength = "Moderate"
	StrengthSupporting Strength = "Supporting"
)

type Direction string

const (
	DirectionPathogenic Direction = "pathogenic"
	DirectionBenign     Direction = "benign"
)

type CriterionStatus string

const (
	CriterionStatusMet           CriterionStatus = "met"
	CriterionStatusNotMet        CriterionStatus = "not_met"
	CriterionStatusNotApplicable CriterionStatus = "not_applicable"
)

type CriterionDefinition struct {
	Code            CriterionCode `json:"code"`
	DefaultStrength Strength      `json:"default_strength"`
	Direction       Direction     `json:"direction"`
	Description     string        `json:"description"`
}

type AppliedCriterion struct {
	Code              CriterionCode   `json:"code"`
	EvaluatedStrength Strength        `json:"evaluated_strength"`
	Direction         Direction       `json:"direction"`
	Status            CriterionStatus `json:"status"`
	Rationale         string          `json:"rationale"`
	SourceIDs         []string        `json:"source_ids,omitempty"`
	Facts             map[string]bool `json:"facts,omitempty"`
}

var (
	ErrUnknownCriterion  = errors.New("unknown ACMG 2015 criterion code")
	ErrDirectionMismatch = errors.New("cannot assign benign strength to pathogenic criterion or vice versa")
)

var CanonicalCriteria = map[CriterionCode]CriterionDefinition{
	// Pathogenic
	CodePVS1: {Code: CodePVS1, DefaultStrength: StrengthVeryStrong, Direction: DirectionPathogenic, Description: "Null variant in a gene where LOF is a known disease mechanism"},
	CodePS1:  {Code: CodePS1, DefaultStrength: StrengthStrong, Direction: DirectionPathogenic, Description: "Same amino acid change as a previously established pathogenic variant"},
	CodePS2:  {Code: CodePS2, DefaultStrength: StrengthStrong, Direction: DirectionPathogenic, Description: "De novo (both maternity and paternity confirmed)"},
	CodePS3:  {Code: CodePS3, DefaultStrength: StrengthStrong, Direction: DirectionPathogenic, Description: "Well-established in vitro or in vivo functional studies supportive of a damaging effect"},
	CodePS4:  {Code: CodePS4, DefaultStrength: StrengthStrong, Direction: DirectionPathogenic, Description: "The prevalence of the variant in affected individuals is significantly increased compared with controls"},
	CodePM1:  {Code: CodePM1, DefaultStrength: StrengthModerate, Direction: DirectionPathogenic, Description: "Located in a mutational hot spot and/or critical and well-established functional domain"},
	CodePM2:  {Code: CodePM2, DefaultStrength: StrengthModerate, Direction: DirectionPathogenic, Description: "Absent from controls or extremely low frequency in population databases"},
	CodePM3:  {Code: CodePM3, DefaultStrength: StrengthModerate, Direction: DirectionPathogenic, Description: "For recessive disorders, detected in trans with a pathogenic variant"},
	CodePM4:  {Code: CodePM4, DefaultStrength: StrengthModerate, Direction: DirectionPathogenic, Description: "Protein length changes as a result of in-frame deletions/insertions or stop-loss"},
	CodePM5:  {Code: CodePM5, DefaultStrength: StrengthModerate, Direction: DirectionPathogenic, Description: "Novel missense change at an amino acid residue where a different pathogenic missense change has been seen"},
	CodePM6:  {Code: CodePM6, DefaultStrength: StrengthModerate, Direction: DirectionPathogenic, Description: "Assumed de novo, but without confirmation of paternity and maternity"},
	CodePP1:  {Code: CodePP1, DefaultStrength: StrengthSupporting, Direction: DirectionPathogenic, Description: "Cosegregation with disease in multiple affected family members"},
	CodePP2:  {Code: CodePP2, DefaultStrength: StrengthSupporting, Direction: DirectionPathogenic, Description: "Missense variant in a gene that has a low rate of benign missense and common pathogenic missense"},
	CodePP3:  {Code: CodePP3, DefaultStrength: StrengthSupporting, Direction: DirectionPathogenic, Description: "Multiple lines of computational evidence support a deleterious effect"},
	CodePP4:  {Code: CodePP4, DefaultStrength: StrengthSupporting, Direction: DirectionPathogenic, Description: "Patient's phenotype or family history is highly specific for a disease with a single genetic etiology"},
	CodePP5:  {Code: CodePP5, DefaultStrength: StrengthSupporting, Direction: DirectionPathogenic, Description: "Reputable source recently reports variant as pathogenic without evidence accessible to evaluate"},

	// Benign
	CodeBA1: {Code: CodeBA1, DefaultStrength: StrengthStandAlone, Direction: DirectionBenign, Description: "Allele frequency is >5% in population databases"},
	CodeBS1: {Code: CodeBS1, DefaultStrength: StrengthStrong, Direction: DirectionBenign, Description: "Allele frequency is greater than expected for disorder"},
	CodeBS2: {Code: CodeBS2, DefaultStrength: StrengthStrong, Direction: DirectionBenign, Description: "Observed in a healthy adult individual for a penetrant dominant disorder"},
	CodeBS3: {Code: CodeBS3, DefaultStrength: StrengthStrong, Direction: DirectionBenign, Description: "Well-established in vitro or in vivo functional studies show no damaging effect"},
	CodeBS4: {Code: CodeBS4, DefaultStrength: StrengthStrong, Direction: DirectionBenign, Description: "Lack of segregation in affected family members"},
	CodeBP1: {Code: CodeBP1, DefaultStrength: StrengthSupporting, Direction: DirectionBenign, Description: "Missense variant in a gene for which primarily truncating variants cause disease"},
	CodeBP2: {Code: CodeBP2, DefaultStrength: StrengthSupporting, Direction: DirectionBenign, Description: "Observed in trans with a pathogenic variant for a dominant disorder, or in cis"},
	CodeBP3: {Code: CodeBP3, DefaultStrength: StrengthSupporting, Direction: DirectionBenign, Description: "In-frame deletions/insertions in a repetitive region without a known function"},
	CodeBP4: {Code: CodeBP4, DefaultStrength: StrengthSupporting, Direction: DirectionBenign, Description: "Multiple lines of computational evidence suggest no impact on gene or product"},
	CodeBP5: {Code: CodeBP5, DefaultStrength: StrengthSupporting, Direction: DirectionBenign, Description: "Variant found in a case with an alternate molecular cause for disease"},
	CodeBP6: {Code: CodeBP6, DefaultStrength: StrengthSupporting, Direction: DirectionBenign, Description: "Reputable source recently reports variant as benign without accessible evidence"},
	CodeBP7: {Code: CodeBP7, DefaultStrength: StrengthSupporting, Direction: DirectionBenign, Description: "A synonymous variant for which splicing prediction algorithms predict no impact"},
}

func LookupCriterion(code string) (CriterionDefinition, error) {
	upper := CriterionCode(strings.ToUpper(strings.TrimSpace(code)))
	def, ok := CanonicalCriteria[upper]
	if !ok {
		return CriterionDefinition{}, fmt.Errorf("%w: %s", ErrUnknownCriterion, code)
	}
	return def, nil
}
