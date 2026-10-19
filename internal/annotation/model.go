// Package annotation normalizes structured variant annotations from ClinVar,
// gnomAD, AlphaMissense, Pangolin and GenCC. It never turns an absent record
// into benign evidence: "not found" and "source unavailable" stay distinct.
package annotation

import (
	"fmt"
	"math"
	"strings"
)

type Status string

const (
	StatusFound       Status = "found"
	StatusNotFound    Status = "not_found"
	StatusConflicting Status = "conflicting"
	StatusUnavailable Status = "unavailable"
	StatusStale       Status = "stale"
	StatusCallerInput Status = "caller_supplied"
)

// Source identifies where an annotation set came from.
type Source string

const (
	SourceCaller   Source = "caller_supplied"
	SourceSnapshot Source = "sepilens_snapshot"
	SourceAPI      Source = "sepilens_api"
)

func (s Source) String() string { return string(s) }

// Record is the common provenance envelope carried by every annotation.
type Record struct {
	RecordHash  string `json:"record_hash,omitempty"`
	Query       *Query `json:"query,omitempty"`
	Source      Source `json:"source"`
	Version     string `json:"version"`
	Assembly    string `json:"assembly,omitempty"`
	RetrievedAt string `json:"retrieved_at,omitempty"`
	Status      Status `json:"status"`
	// Reason is a short, non-clinical explanation for a non-found status.
	Reason string `json:"reason,omitempty"`
}

type Set struct {
	Source        Source         `json:"source"`
	ClinVar       *ClinVar       `json:"clinvar,omitempty"`
	GnomAD        *GnomAD        `json:"gnomad,omitempty"`
	AlphaMissense *AlphaMissense `json:"alphamissense,omitempty"`
	Pangolin      *Pangolin      `json:"pangolin,omitempty"`
	GenCC         *GenCC         `json:"gencc,omitempty"`
}

// MarkCallerSupplied discards trust claims arriving over the public API.
func (s *Set) MarkCallerSupplied() {
	s.Source = SourceCaller
	for _, r := range []*Record{recClinVar(s.ClinVar), recGnomAD(s.GnomAD), recAlpha(s.AlphaMissense), recPangolin(s.Pangolin), recGenCC(s.GenCC)} {
		if r != nil {
			r.Source = SourceCaller
		}
	}
}

// ClinVar carries the fields needed for PS1/PM5 and disease consistency checks.
type ClinVar struct {
	Record
	Transcript     string   `json:"transcript,omitempty"`
	VariationID    string   `json:"variation_id,omitempty"`
	AlleleID       string   `json:"allele_id,omitempty"`
	Classification string   `json:"classification,omitempty"`
	ReviewStatus   string   `json:"review_status,omitempty"`
	LastEvaluated  string   `json:"last_evaluated,omitempty"`
	Conditions     []string `json:"conditions,omitempty"`
	HGVSc          string   `json:"hgvs_c,omitempty"`
	HGVSp          string   `json:"hgvs_p,omitempty"`
	SubmitterCount int      `json:"submitter_count,omitempty"`
	Conflicting    bool     `json:"conflicting,omitempty"`
	// OtherPathogenicSameAminoAcid lists distinct nucleotide changes that produce
	// the same protein change and are classified pathogenic. Required for PS1.
	OtherPathogenicSameAminoAcid []string `json:"other_pathogenic_same_amino_acid,omitempty"`
	// OtherPathogenicSameResidue lists pathogenic missense changes at the same
	// residue with a different substitution. Required for PM5.
	OtherPathogenicSameResidue []string `json:"other_pathogenic_same_residue,omitempty"`
}

// GnomAD carries allele frequency and coverage. PM2 requires FAF95 plus
// adequate coverage; a missing record is not rarity evidence.
type GnomAD struct {
	Record
	PopMaxAF         *float64 `json:"popmax_af,omitempty"`
	PopMaxPopulation string   `json:"popmax_population,omitempty"`
	FAF95            *float64 `json:"faf95,omitempty"`
	AlleleCount      *int     `json:"allele_count,omitempty"`
	AlleleNumber     *int     `json:"allele_number,omitempty"`
	HomozygoteCount  *int     `json:"homozygote_count,omitempty"`
	HemizygoteCount  *int     `json:"hemizygote_count,omitempty"`
	CoverageAdequate *bool    `json:"coverage_adequate,omitempty"`
}

type AlphaMissense struct {
	Record
	Score      *float64 `json:"score,omitempty"`
	Class      string   `json:"class,omitempty"` // likely_benign, ambiguous, likely_pathogenic
	Transcript string   `json:"transcript,omitempty"`
	Protein    string   `json:"protein_change,omitempty"`
}

type Pangolin struct {
	Record
	Score          *float64 `json:"score,omitempty"`
	DeltaScore     *float64 `json:"delta_score,omitempty"`
	SpliceSite     string   `json:"splice_site,omitempty"`
	Interpretation string   `json:"interpretation,omitempty"` // splice_altering, no_splice_impact, uncertain
	Transcript     string   `json:"transcript,omitempty"`
}

type GenCC struct {
	Record
	Gene           string `json:"gene,omitempty"`
	DiseaseID      string `json:"disease_id,omitempty"`
	DiseaseName    string `json:"disease_name,omitempty"`
	Classification string `json:"classification,omitempty"` // Definitive, Strong, Moderate, Limited, Disputed, Refuted
	MOI            string `json:"moi,omitempty"`
	Submitter      string `json:"submitter,omitempty"`
}

var (
	ErrMissingVersion    = fmt.Errorf("annotation version is required")
	ErrAssemblyMismatch  = fmt.Errorf("annotation assembly mismatch")
	ErrInvalidFrequency  = fmt.Errorf("allele frequency out of range")
	ErrInvalidAlphaScore = fmt.Errorf("alphamissense score out of range")
	ErrInvalidPangolin   = fmt.Errorf("pangolin delta_score out of range")
)

// Validate checks structural invariants only. It does not decide clinical meaning.
func (s *Set) Validate(assembly string) error {
	if s == nil {
		return nil
	}
	if s.Source == "" {
		s.Source = SourceCaller
	}
	if s.Source != SourceCaller && s.Source != SourceSnapshot && s.Source != SourceAPI {
		return fmt.Errorf("invalid annotation source")
	}
	check := func(r *Record, name string) error {
		if r == nil {
			return nil
		}
		if r.Version == "" {
			return fmt.Errorf("%s: %w", name, ErrMissingVersion)
		}
		if r.Source != "" && r.Source != SourceCaller && r.Source != SourceSnapshot && r.Source != SourceAPI {
			return fmt.Errorf("%s: invalid annotation source", name)
		}
		switch r.Status {
		case "", StatusFound, StatusNotFound, StatusConflicting, StatusUnavailable, StatusStale, StatusCallerInput:
		default:
			return fmt.Errorf("%s: invalid annotation status", name)
		}
		if r.Assembly != "" && normalizeAssembly(r.Assembly) != normalizeAssembly(assembly) {
			return fmt.Errorf("%s: %w", name, ErrAssemblyMismatch)
		}
		if r.Status == "" {
			r.Status = StatusFound
		}
		return nil
	}
	for name, record := range map[string]*Record{
		"clinvar":       recClinVar(s.ClinVar),
		"gnomad":        recGnomAD(s.GnomAD),
		"alphamissense": recAlpha(s.AlphaMissense),
		"pangolin":      recPangolin(s.Pangolin),
		"gencc":         recGenCC(s.GenCC),
	} {
		if err := check(record, name); err != nil {
			return err
		}
	}
	if g := s.GnomAD; g != nil {
		for _, af := range []*float64{g.PopMaxAF, g.FAF95} {
			if af != nil && (*af < 0 || *af > 1 || math.IsNaN(*af) || math.IsInf(*af, 0)) {
				return fmt.Errorf("gnomad: %w", ErrInvalidFrequency)
			}
		}
	}
	if a := s.AlphaMissense; a != nil && a.Score != nil {
		if *a.Score < 0 || *a.Score > 1 || math.IsNaN(*a.Score) {
			return fmt.Errorf("alphamissense: %w", ErrInvalidAlphaScore)
		}
	}
	if p := s.Pangolin; p != nil && p.DeltaScore != nil {
		if *p.DeltaScore < 0 || *p.DeltaScore > 1 || math.IsNaN(*p.DeltaScore) {
			return fmt.Errorf("pangolin: %w", ErrInvalidPangolin)
		}
	}
	return nil
}

func recClinVar(v *ClinVar) *Record {
	if v == nil {
		return nil
	}
	return &v.Record
}

func recGnomAD(v *GnomAD) *Record {
	if v == nil {
		return nil
	}
	return &v.Record
}

func recAlpha(v *AlphaMissense) *Record {
	if v == nil {
		return nil
	}
	return &v.Record
}

func recPangolin(v *Pangolin) *Record {
	if v == nil {
		return nil
	}
	return &v.Record
}

func recGenCC(v *GenCC) *Record {
	if v == nil {
		return nil
	}
	return &v.Record
}

func normalizeAssembly(v string) string {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "GRCH37", "HG19":
		return "GRCh37"
	case "GRCH38", "HG38":
		return "GRCh38"
	default:
		return strings.TrimSpace(v)
	}
}

// ReviewStars maps ClinVar review status to the conventional 0-4 star scale used
// by PS1/PM5 gates. Only statuses that imply external review score above zero.
func (c *ClinVar) ReviewStars() int {
	if c == nil {
		return 0
	}
	switch strings.ReplaceAll(strings.ToLower(strings.TrimSpace(c.ReviewStatus)), " ", "_") {
	case "practice_guideline", "reviewed_by_expert_panel":
		return 3
	case "criteria_provided,_multiple_submitters,_no_conflicts":
		return 2
	case "criteria_provided,_single_submitter":
		return 1
	case "criteria_provided,_conflicting_classifications", "conflicting":
		return 0
	default:
		return 0
	}
}

// IsPathogenic reports a curated pathogenic or likely pathogenic call.
func (c *ClinVar) IsPathogenic() bool {
	if c == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(c.Classification)) {
	case "pathogenic", "likely pathogenic", "pathogenic/likely pathogenic":
		return true
	}
	return false
}

// IsBenign reports a curated benign or likely benign call.
func (c *ClinVar) IsBenign() bool {
	if c == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(c.Classification)) {
	case "benign", "likely benign", "benign/likely benign":
		return true
	}
	return false
}

// CoverageConfident requires an explicit positive coverage flag and a minimum
// allele number. Absent data never counts as adequate.
func (g *GnomAD) CoverageConfident(minAlleleNumber int) bool {
	if g == nil || g.CoverageAdequate == nil || !*g.CoverageAdequate {
		return false
	}
	if g.AlleleNumber == nil {
		return false
	}
	return *g.AlleleNumber >= minAlleleNumber
}

// Rarity returns the frequency used for rarity-based criteria, preferring FAF95
// and falling back to popmax only when FAF95 is absent. ok=false means no usable
// frequency was supplied.
func (g *GnomAD) Rarity() (float64, bool) {
	if g == nil {
		return 0, false
	}
	if g.FAF95 != nil {
		return *g.FAF95, true
	}
	if g.PopMaxAF != nil {
		return *g.PopMaxAF, true
	}
	return 0, false
}

// FrequencyForCommonVariant returns popmax AF for BA1-style checks.
func (g *GnomAD) FrequencyForCommonVariant() (float64, bool) {
	if g == nil || g.PopMaxAF == nil {
		return 0, false
	}
	return *g.PopMaxAF, true
}

// GenCCSupports reports whether the classification can support a gene-disease
// assertion. Disputed and Refuted never do.
func (g *GenCC) GenCCSupports() bool {
	if g == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(g.Classification)) {
	case "definitive", "strong":
		return true
	}
	return false
}

// RefutedOrDisputed reports a gene-disease assertion that blocks classification.
func (g *GenCC) RefutedOrDisputed() bool {
	if g == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(g.Classification)) {
	case "refuted", "disputed":
		return true
	}
	return false
}

// HasRecord reports whether a source returned a usable record.
func HasRecord(status Status) bool {
	return status == StatusFound || status == StatusCallerInput
}
