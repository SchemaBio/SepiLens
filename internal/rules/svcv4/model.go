// Package svcv4 implements the locked, NON-AUTHORITATIVE ClinGen draft reference scorer.
// It does not implement a clinically approved SVC v4 engine. See UPSTREAM_LICENSE.
package svcv4

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"strings"
)

const Commit = "ef66faff51a265fef7b5c4e6439905f3aa540c46"
const Version = "draft-reference-" + Commit
const Unit int64 = 1000000

// Point is exact fixed-point arithmetic in millionths. JSON accepts numbers or decimals.
type Point int64

func (p *Point) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if !decimalToken(s) {
		return errors.New("invalid decimal points")
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return errors.New("invalid decimal points")
	}
	r.Mul(r, big.NewRat(Unit, 1))
	if !r.IsInt() || !r.Num().IsInt64() || r.Num().Int64() < -1000*Unit || r.Num().Int64() > 1000*Unit {
		return errors.New("points require at most six decimals within -1000..1000")
	}
	*p = Point(r.Num().Int64())
	return nil
}
func (p Point) MarshalJSON() ([]byte, error) { return json.Marshal(p.String()) }
func (p Point) String() string {
	n := int64(p)
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	return sign + fmt.Sprintf("%d", n/Unit) + strings.TrimRight(strings.TrimRight(fmt.Sprintf(".%06d", n%Unit), "0"), ".")
}
func pt(n int64) Point   { return Point(n * Unit) }
func ptr(p Point) *Point { return &p }

type DraftAssessment struct {
	UpstreamCommit      string       `json:"upstream_commit,omitempty"`
	MOI                 string       `json:"moi"`
	GeneDiseaseValidity string       `json:"gene_disease_validity,omitempty"`
	Population          *Population  `json:"population,omitempty"`
	Cases               []Case       `json:"cases,omitempty"`
	CaseControl         *CaseControl `json:"case_control,omitempty"`
	LocusCaseID         string       `json:"locus_case_id,omitempty"`
	Impact              *Impact      `json:"variant_impact,omitempty"`
}
type Population struct {
	FAF             string `json:"faf,omitempty"`
	DAFT            string `json:"daft,omitempty"`
	HMZEligible     string `json:"hmz_eligible,omitempty"`
	HomozygoteCount *int   `json:"homozygote_count,omitempty"`
	HemizygoteCount *int   `json:"hemizygote_count,omitempty"`
}
type Testing struct {
	DiagnosticYield    string `json:"diagnostic_yield_for_phenotypes,omitempty"`
	CoversAllGenes     string `json:"covers_all_genes_relevant_to_mde,omitempty"`
	NonGeneticExcluded string `json:"non_genetic_etiology_excluded,omitempty"`
}
type OtherVariant struct {
	Classification  string `json:"classification,omitempty"`
	Phase           string `json:"phase_in_ref_to_vbc,omitempty"`
	PhaseConfidence string `json:"phase_confidence,omitempty"`
	CoOccurrence    string `json:"co_occurrence_likelihood,omitempty"`
}
type Relative struct {
	Parent      string `json:"parent_of_proband,omitempty"`
	Affected    string `json:"affected_w_mde,omitempty"`
	Exists      string `json:"vbc_exists,omitempty"`
	Zygosity    string `json:"vbc_zygosity,omitempty"`
	CompoundHet string `json:"cmp_het_variant_exists,omitempty"`
	Severe      string `json:"severe_phenotype,omitempty"`
}
type Case struct {
	ID          string         `json:"id"`
	FamilyID    string         `json:"family_id"`
	Sex         string         `json:"sex,omitempty"`
	Phenotype   string         `json:"pheno_specificity_for_mde,omitempty"`
	Severity    string         `json:"pheno_severity,omitempty"`
	Penetrance  string         `json:"age_matched_penetrance,omitempty"`
	Parentage   string         `json:"confirmed_parental_relationship,omitempty"`
	Zygosity    string         `json:"vbc_zygosity,omitempty"`
	Testing     *Testing       `json:"testing,omitempty"`
	CompoundHet *OtherVariant  `json:"compound_het_variant,omitempty"`
	Additional  []OtherVariant `json:"additional_variants,omitempty"`
	Relatives   []Relative     `json:"relatives,omitempty"`
}
type CaseControl struct {
	OddsRatio       string `json:"odds_ratio,omitempty"`
	CILower         string `json:"ci_lower,omitempty"`
	CIUpper         string `json:"ci_upper,omitempty"`
	VariantCount    int    `json:"case_variant_count"`
	CohortSize      int    `json:"case_cohort_size"`
	ControlsMatched bool   `json:"controls_matched"`
	BiasConsidered  bool   `json:"ascertainment_bias_considered"`
}
type Informative struct {
	ID             string `json:"id"`
	Classification string `json:"classification,omitempty"`
	Category       string `json:"category,omitempty"`
}
type Impact struct {
	Workflow    string        `json:"workflow"`
	Outcome     string        `json:"prediction_outcome,omitempty"`
	Initial     *Point        `json:"initial_points,omitempty"`
	Mechanism   string        `json:"gencc_mechanism,omitempty"`
	Exon        string        `json:"exon_relevance,omitempty"`
	Functional  *Point        `json:"fxn_points,omitempty"`
	SpliceAssay *Point        `json:"spa_points,omitempty"`
	Informative []Informative `json:"informative,omitempty"`
	AminoAcid   *Impact       `json:"amino_acid,omitempty"`
	Splice      *Impact       `json:"splice,omitempty"`
}
type FamilyResult struct {
	ComparedPaths []FamilyResult   `json:"compared_paths,omitempty"`
	SelectedPath  string           `json:"selected_path,omitempty"`
	ParentCode    string           `json:"parent_code"`
	SubCodePoints map[string]Point `json:"sub_code_points"`
	Held          map[string]Point `json:"held_combined"`
	Total         *Point           `json:"parent_total"`
	Missing       []string         `json:"missing_information"`
}
type DraftResult struct {
	Version        string         `json:"version"`
	UpstreamCommit string         `json:"upstream_commit"`
	RulesetHash    string         `json:"ruleset_hash"`
	Authoritative  bool           `json:"authoritative"`
	InputsVerified bool           `json:"inputs_verified"`
	ClinicalUse    string         `json:"clinical_use"`
	Classification string         `json:"classification"`
	VUSSubclass    string         `json:"vus_subclass,omitempty"`
	Total          *Point         `json:"total_points"`
	Families       []FamilyResult `json:"families"`
	Missing        []string       `json:"missing_information"`
	Assumptions    []string       `json:"assumptions"`
}

func oneOf(s string, values ...string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}
func number(s string) (*big.Rat, error) {
	if !decimalToken(s) {
		return nil, errors.New("invalid decimal token")
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok || r.Sign() < 0 {
		return nil, errors.New("nonnegative decimal required")
	}
	return r, nil
}

var decimalPattern = regexp.MustCompile(`^[+-]?[0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]{1,3})?$`)

func decimalToken(s string) bool { return len(s) <= 64 && decimalPattern.MatchString(s) }
func Validate(a *DraftAssessment) error {
	if a == nil {
		return errors.New("draft_assessment required")
	}
	if a.UpstreamCommit != "" && a.UpstreamCommit != Commit {
		return errors.New("unsupported draft commit")
	}
	if !oneOf(a.MOI, "AD", "AR", "XLD", "XLR", "SD") {
		return errors.New("draft moi must be AD, AR, XLD, XLR or SD")
	}
	if !oneOf(a.GeneDiseaseValidity, "", "DEFINITIVE", "STRONG", "MODERATE", "LIMITED", "DISPUTED", "REFUTED", "NOT_CLASSIFIED") {
		return errors.New("invalid gene disease validity")
	}
	if len(a.Cases) > 200 {
		return errors.New("at most 200 unrelated probands")
	}
	ids, families := map[string]bool{}, map[string]bool{}
	for _, c := range a.Cases {
		if strings.TrimSpace(c.ID) == "" || strings.TrimSpace(c.FamilyID) == "" || ids[c.ID] || families[c.FamilyID] {
			return errors.New("one distinct index proband per identified family required")
		}
		ids[c.ID] = true
		families[c.FamilyID] = true
		if !oneOf(c.Sex, "", "M", "F", "U", "T") || !oneOf(c.Phenotype, "", "SPECIFIC", "CONSISTENT", "INCONSISTENT") || !oneOf(c.Penetrance, "", "LT_80", "PCT_80_100", "NEAR_100") || !oneOf(c.Zygosity, "", "HOM", "HET", "HEMI") || !oneOf(c.Parentage, "", "TRUE", "FALSE", "UNKNOWN") || !oneOf(c.Severity, "", "MONO_GT_OR_BIALLELIC_EQ_EXPECTED", "MONO_EQ_EXPECTED", "BIALLELIC_LT_EXPECTED") {
			return errors.New("invalid case category")
		}
		if c.Testing != nil {
			if !oneOf(c.Testing.CoversAllGenes, "", "TRUE", "FALSE", "UNKNOWN") || !oneOf(c.Testing.NonGeneticExcluded, "", "TRUE", "FALSE", "UNKNOWN") {
				return errors.New("invalid testing category")
			}
			if c.Testing.DiagnosticYield != "" {
				if _, err := yieldPercent(c.Testing.DiagnosticYield); err != nil {
					return err
				}
			}
		}
		variants := append([]OtherVariant{}, c.Additional...)
		if c.CompoundHet != nil {
			variants = append(variants, *c.CompoundHet)
		}
		for _, v := range variants {
			if !oneOf(v.Classification, "", "P", "LP", "VUS", "B", "LB", "PATHOGENIC", "LIKELY_PATHOGENIC", "BENIGN", "LIKELY_BENIGN") || !oneOf(v.Phase, "", "CIS", "TRANS", "UNKNOWN") || !oneOf(v.PhaseConfidence, "", "HIGH", "MED", "LOW") || !oneOf(v.CoOccurrence, "", "LT_0_0001", "BETWEEN_0_0001_0_01", "NOT_ASSESSED") {
				return errors.New("invalid additional variant")
			}
		}
		if len(c.Relatives) > 1000 || len(c.Additional) > 100 {
			return errors.New("case exceeds reference input limit")
		}
		for _, r := range c.Relatives {
			for _, s := range []string{r.Parent, r.Affected, r.Exists, r.CompoundHet, r.Severe} {
				if !oneOf(s, "", "TRUE", "FALSE", "UNKNOWN") {
					return errors.New("invalid relative tristate")
				}
			}
			if !oneOf(r.Zygosity, "", "HOM", "HET", "HEMI") {
				return errors.New("invalid relative zygosity")
			}
		}
	}
	if a.LocusCaseID != "" && !ids[a.LocusCaseID] {
		return errors.New("locus case is not an input proband")
	}
	if p := a.Population; p != nil {
		if !oneOf(p.HMZEligible, "", "TRUE", "FALSE", "UNKNOWN") {
			return errors.New("invalid hmz eligibility")
		}
		for _, s := range []string{p.FAF, p.DAFT} {
			if s != "" {
				r, e := number(s)
				if e != nil || r.Cmp(big.NewRat(1, 1)) > 0 {
					return errors.New("frequency must be a decimal in 0..1")
				}
			}
		}
		for _, n := range []*int{p.HomozygoteCount, p.HemizygoteCount} {
			if n != nil && (*n < 0 || *n > 1000000) {
				return errors.New("invalid occurrence count")
			}
		}
	}
	if c := a.CaseControl; c != nil {
		for _, s := range []string{c.OddsRatio, c.CILower, c.CIUpper} {
			if s != "" {
				if _, e := number(s); e != nil {
					return e
				}
			}
		}
		if c.VariantCount < 0 || c.CohortSize < 0 || c.VariantCount > c.CohortSize {
			return errors.New("invalid case control counts")
		}
		if c.CILower != "" && c.CIUpper != "" {
			l, _ := number(c.CILower)
			h, _ := number(c.CIUpper)
			if l.Cmp(h) > 0 {
				return errors.New("invalid confidence interval")
			}
		}
	}
	if e := validateImpact(a.Impact, 0); e != nil {
		return e
	}
	a.UpstreamCommit = Commit
	return nil
}
func validateImpact(a *Impact, depth int) error {
	if a == nil {
		return nil
	}
	if depth > 1 {
		return errors.New("nested impact workflow not allowed")
	}
	if a.Workflow == "missense" {
		if depth != 0 || a.Initial != nil || a.Functional != nil || a.SpliceAssay != nil || len(a.Informative) > 0 || a.Outcome != "" {
			return errors.New("missense requires separate amino acid and splice assessments")
		}
		if a.AminoAcid != nil && a.AminoAcid.Workflow != "missense_amino_acid" {
			return errors.New("invalid amino acid workflow")
		}
		if a.Splice != nil && a.Splice.Workflow != "missense_splice" {
			return errors.New("invalid missense splice workflow")
		}
		if e := validateImpact(a.AminoAcid, depth+1); e != nil {
			return e
		}
		return validateImpact(a.Splice, depth+1)
	}
	if a.AminoAcid != nil || a.Splice != nil {
		return errors.New("unexpected nested workflow")
	}
	if !oneOf(a.Mechanism, "", "ESTABLISHED", "LIKELY", "SUSPECTED", "UNCERTAIN") || !oneOf(a.Exon, "", "ALL", "MOST", "FEW") {
		return errors.New("invalid mechanism or exon relevance")
	}
	if a.Initial != nil && *a.Initial > 0 {
		denominator := int64(1)
		if a.Workflow == "missense_amino_acid" {
			if a.Exon == "MOST" {
				denominator = 2
			}
		} else if !(a.Workflow == "inframe_indel" && a.Outcome == "SIMPLE_SEQUENCE_REPEAT") {
			switch a.Mechanism {
			case "LIKELY":
				denominator = 2
			case "SUSPECTED":
				denominator = 4
			}
			if a.Exon == "MOST" && !(a.Workflow == "exon_deletion" && a.Outcome == "WHOLE_GENE") {
				denominator *= 2
			}
		}
		if int64(*a.Initial)%denominator != 0 {
			return errors.New("point multiplier would exceed supported exact decimal precision")
		}
	}
	if a.Workflow == "missense_amino_acid" {
		if a.Outcome != "" {
			return errors.New("amino acid workflow has no branch")
		}
	} else if a.Workflow == "inframe_indel" {
		if !oneOf(a.Outcome, "SIMPLE_SEQUENCE_REPEAT", "NON_REPEAT") {
			return errors.New("unsupported inframe branch")
		}
	} else if a.Workflow == "exon_duplication" && a.Outcome == "WHOLE_GENE_NA" {
	} else if _, ok := branchTable[a.Workflow][a.Outcome]; !ok {
		return errors.New("unsupported impact workflow or prediction outcome")
	}
	if len(a.Informative) > 1000 {
		return errors.New("too many informative variants")
	}
	ids := map[string]bool{}
	for _, v := range a.Informative {
		if v.ID == "" || ids[v.ID] {
			return errors.New("informative variant identifiers must be distinct")
		}
		ids[v.ID] = true
		if !oneOf(v.Classification, "", "PATHOGENIC", "LIKELY_PATHOGENIC", "VUS", "BENIGN", "LIKELY_BENIGN") {
			return errors.New("invalid informative classification")
		}
		if a.Workflow == "missense_amino_acid" && !oneOf(v.Category, "", "SAME_AA_PATHOGENIC", "DISTINCT_AA_PATHOGENIC", "DISTINCT_AA_BENIGN", "SAME_AA_BENIGN") {
			return errors.New("invalid missense informative category")
		}
	}
	return nil
}

// Decode also rejects unknown fields and trailing JSON, including nested assessments.
func Decode(b []byte) (*DraftAssessment, error) {
	var a DraftAssessment
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e := d.Decode(&a); e != nil {
		return nil, e
	}
	var trailing any
	if d.Decode(&trailing) != io.EOF {
		return nil, errors.New("trailing JSON")
	}
	return &a, Validate(&a)
}
