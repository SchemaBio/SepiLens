package acmg2015

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pzweuj/sepilens/internal/annotation"
)

// DeriveOptions controls optional criteria. Defaults stay conservative.
type DeriveOptions struct {
	AllowPP5BP6 bool
	// PVS1MechanismConfirmed is an attestation that LOF is an established disease
	// mechanism for this gene-disease pair. GenCC validity alone is not enough.
	PVS1MechanismConfirmed bool
	// TranscriptRelevant is an attestation that the transcript is the clinical one.
	TranscriptRelevant bool
}

// Candidate is a derived criterion with its provenance and unmet prerequisites.
type Candidate struct {
	Code               CriterionCode   `json:"code"`
	Direction          Direction       `json:"direction"`
	Status             CriterionStatus `json:"status"`
	Strength           Strength        `json:"strength"`
	Reason             string          `json:"reason"`
	Facts              map[string]bool `json:"facts,omitempty"`
	SourceIDs          []string        `json:"source_ids,omitempty"`
	MissingInformation []string        `json:"missing_information,omitempty"`
}

// Derivation is the deterministic result of applying annotations to criteria.
type Derivation struct {
	Candidates []Candidate `json:"candidates"`
	Gaps       []string    `json:"gaps,omitempty"`
	Conflicts  []string    `json:"conflicts,omitempty"`
}

const (
	// pm2SupportingStrength and bp4SupportingStrength encode the SVI recommendation
	// that rarity and computational evidence default to Supporting until calibrated.
	pm2SupportingStrength = StrengthSupporting
	ba1FrequencyCutoff    = 0.05
	minGnomADAlleleNumber = 1000
)

// Derive maps normalized annotations to ACMG criteria. It never invents a fact:
// when a prerequisite is missing the candidate is insufficient_data.
func Derive(a *annotation.Set, variant SNVVariantInput, ctx ClinicalContextInput, opts DeriveOptions) Derivation {
	var out Derivation
	if a == nil {
		out.Gaps = append(out.Gaps, "no annotations supplied")
		return out
	}
	out.add(derivePM2(a.GnomAD))
	out.add(deriveBA1(a.GnomAD))
	out.add(derivePM4(variant))
	out.add(derivePP3(a.AlphaMissense, a.Pangolin, variant))
	out.add(deriveBP4(a.AlphaMissense, a.Pangolin, variant))
	out.add(deriveBP7(variant, a.Pangolin))
	out.add(derivePS1(a.ClinVar))
	out.add(derivePM5(a.ClinVar))
	out.add(deriveBP1(variant, ctx))
	if opts.AllowPP5BP6 {
		out.add(derivePP5(a.ClinVar))
		out.add(deriveBP6(a.ClinVar))
	}
	out.add(derivePVS1(variant, a, ctx, opts))
	return out
}

func (d *Derivation) add(c *Candidate) {
	if c == nil {
		return
	}
	d.Candidates = append(d.Candidates, *c)
	sort.SliceStable(d.Candidates, func(i, j int) bool { return d.Candidates[i].Code < d.Candidates[j].Code })
}

func met(code CriterionCode, strength Strength, reason string, facts map[string]bool, sources ...string) *Candidate {
	def := CanonicalCriteria[code]
	return &Candidate{Code: code, Direction: def.Direction, Status: CriterionStatusMet, Strength: strength, Reason: reason, Facts: facts, SourceIDs: sources}
}

func insufficient(code CriterionCode, reason string, missing ...string) *Candidate {
	return &Candidate{Code: code, Direction: CanonicalCriteria[code].Direction, Status: CriterionStatusInsufficientData, Strength: CanonicalCriteria[code].DefaultStrength, Reason: reason, MissingInformation: missing}
}

func notApplicable(code CriterionCode, reason string) *Candidate {
	return &Candidate{Code: code, Direction: CanonicalCriteria[code].Direction, Status: CriterionStatusNotApplicable, Strength: CanonicalCriteria[code].DefaultStrength, Reason: reason}
}

// notMet means the criterion was evaluated and the evidence did not satisfy it.
// This differs from not_applicable (wrong variant class) and insufficient_data
// (a prerequisite is missing).
func notMet(code CriterionCode, reason string) *Candidate {
	return &Candidate{Code: code, Direction: CanonicalCriteria[code].Direction, Status: CriterionStatusNotMet, Strength: CanonicalCriteria[code].DefaultStrength, Reason: reason}
}

// derivePM2 requires FAF95, adequate coverage and rarity. SVI guidance downgrades
// PM2 to Supporting; the older Moderate assumption is not used.
func derivePM2(g *annotation.GnomAD) *Candidate {
	if g == nil || g.Status != annotation.StatusFound && g.Status != annotation.StatusCallerInput {
		return insufficient(CodePM2, "gnomAD record unavailable", "population_coverage_adequate")
	}
	if !g.CoverageConfident(minGnomADAlleleNumber) {
		return insufficient(CodePM2, "population coverage is not adequate for rarity assessment", "population_coverage_adequate")
	}
	af, ok := g.Rarity()
	if !ok {
		return insufficient(CodePM2, "no usable FAF95 or popmax frequency", "absent_or_moi_appropriate_rarity")
	}
	if af >= ba1FrequencyCutoff {
		return notMet(CodePM2, "frequency is too high for PM2")
	}
	return met(CodePM2, pm2SupportingStrength, "gnomAD FAF95 is rare with adequate coverage; SVI default strength is Supporting",
		map[string]bool{"population_coverage_adequate": true, "absent_or_moi_appropriate_rarity": af < 0.0001},
		"gnomad:"+g.Version)
}

func deriveBA1(g *annotation.GnomAD) *Candidate {
	if g == nil || (g.Status != annotation.StatusFound && g.Status != annotation.StatusCallerInput) {
		return insufficient(CodeBA1, "gnomAD record unavailable", "population_coverage_adequate")
	}
	af, ok := g.FrequencyForCommonVariant()
	if !ok {
		return insufficient(CodeBA1, "popmax allele frequency missing", "frequency_above_five_percent")
	}
	if !g.CoverageConfident(minGnomADAlleleNumber) {
		return insufficient(CodeBA1, "population coverage is not adequate", "population_coverage_adequate")
	}
	if af <= ba1FrequencyCutoff {
		return notMet(CodeBA1, "popmax allele frequency is not above 5%")
	}
	return met(CodeBA1, StrengthStandAlone, "popmax allele frequency exceeds 5% with adequate coverage",
		map[string]bool{"frequency_above_five_percent": true, "population_coverage_adequate": true, "ba1_exceptions_excluded": false},
		"gnomad:"+g.Version)
}

func derivePM4(v SNVVariantInput) *Candidate {
	consequence := strings.ToLower(strings.TrimSpace(v.Consequence))
	inframe := strings.Contains(consequence, "inframe_insertion") || strings.Contains(consequence, "inframe_deletion") || strings.Contains(consequence, "stop_lost")
	if !inframe {
		return notApplicable(CodePM4, "consequence is not an in-frame length change")
	}
	// Splice impact must be assessed; deriving it needs the Pangolin record.
	return insufficient(CodePM4, "protein length change requires splice impact assessment", "splice_impact_assessed")
}

func derivePP3(am *annotation.AlphaMissense, pg *annotation.Pangolin, v SNVVariantInput) *Candidate {
	var sources []string
	missense := strings.Contains(strings.ToLower(v.Consequence), "missense") || strings.TrimSpace(v.HGVSp) != ""
	support := false
	if am != nil && am.Class == "likely_pathogenic" && missense && transcriptMatches(am.Transcript, v.Transcript) {
		support = true
		sources = append(sources, "alphamissense:"+am.Version)
	}
	if pg != nil && pg.Interpretation == "splice_altering" && transcriptMatches(pg.Transcript, v.Transcript) {
		support = true
		sources = append(sources, "pangolin:"+pg.Version)
	}
	if !support {
		return notApplicable(CodePP3, "no applicable computational evidence")
	}
	return met(CodePP3, StrengthSupporting, "computational evidence supports a damaging effect; correlated predictors form one criterion",
		map[string]bool{"computational_support": true, "predictor_dependence_assessed": true}, sources...)
}

func deriveBP4(am *annotation.AlphaMissense, pg *annotation.Pangolin, v SNVVariantInput) *Candidate {
	var sources []string
	missense := strings.Contains(strings.ToLower(v.Consequence), "missense") || strings.TrimSpace(v.HGVSp) != ""
	if am != nil && am.Class == "likely_benign" && missense && transcriptMatches(am.Transcript, v.Transcript) {
		sources = append(sources, "alphamissense:"+am.Version)
	}
	if pg != nil && pg.Interpretation == "no_splice_impact" && transcriptMatches(pg.Transcript, v.Transcript) {
		sources = append(sources, "pangolin:"+pg.Version)
	}
	if len(sources) == 0 {
		return notApplicable(CodeBP4, "no applicable computational evidence")
	}
	return met(CodeBP4, StrengthSupporting, "computational evidence suggests no impact; correlated predictors form one criterion",
		map[string]bool{"computational_no_impact": true, "predictor_dependence_assessed": true}, sources...)
}

func deriveBP7(v SNVVariantInput, pg *annotation.Pangolin) *Candidate {
	consequence := strings.ToLower(strings.TrimSpace(v.Consequence))
	if !strings.Contains(consequence, "synonymous") && !strings.Contains(consequence, "synonymous_variant") {
		return notApplicable(CodeBP7, "variant is not synonymous")
	}
	if pg == nil || pg.Interpretation != "no_splice_impact" || !transcriptMatches(pg.Transcript, v.Transcript) {
		return insufficient(CodeBP7, "synonymous variant requires a splice prediction", "no_splice_impact")
	}
	return insufficient(CodeBP7, "conservation evidence is required and not supplied", "not_highly_conserved")
}

func derivePS1(cv *annotation.ClinVar) *Candidate {
	if cv == nil || len(cv.OtherPathogenicSameAminoAcid) == 0 {
		return insufficient(CodePS1, "no established pathogenic change at the same amino acid", "same_amino_acid_as_established_pathogenic")
	}
	if cv.ReviewStars() < annotation.MinReviewStarsForRelativeCriterion {
		return insufficient(CodePS1, "ClinVar review status is below the PS1 threshold", "same_amino_acid_as_established_pathogenic")
	}
	if cv.Conflicting {
		return &Candidate{Code: CodePS1, Direction: DirectionPathogenic, Status: CriterionStatusConflicting, Strength: StrengthStrong, Reason: "ClinVar classifications conflict at this residue"}
	}
	return met(CodePS1, StrengthStrong, "a different nucleotide change producing the same amino acid change is established pathogenic",
		map[string]bool{"same_amino_acid_as_established_pathogenic": true, "splice_impact_excluded": true},
		"clinvar:"+cv.Version)
}

func derivePM5(cv *annotation.ClinVar) *Candidate {
	if cv == nil || len(cv.OtherPathogenicSameResidue) == 0 {
		return insufficient(CodePM5, "no established pathogenic missense at the same residue", "different_missense_same_residue_pathogenic")
	}
	if cv.ReviewStars() < annotation.MinReviewStarsForRelativeCriterion {
		return insufficient(CodePM5, "ClinVar review status is below the PM5 threshold", "different_missense_same_residue_pathogenic")
	}
	return met(CodePM5, StrengthModerate, "a different pathogenic missense at the same residue is established",
		map[string]bool{"different_missense_same_residue_pathogenic": true, "splice_impact_excluded": true},
		"clinvar:"+cv.Version)
}

func deriveBP1(v SNVVariantInput, ctx ClinicalContextInput) *Candidate {
	if !strings.Contains(strings.ToLower(v.Consequence), "missense") {
		return notApplicable(CodeBP1, "variant is not missense")
	}
	// Disease mechanism is not encoded in the request; requiring an explicit
	// mechanism attestation would need a curated source that is not wired yet.
	return insufficient(CodeBP1, "gene disease mechanism is not established in the supplied context", "primarily_truncating_disease_mechanism")
}

func derivePP5(cv *annotation.ClinVar) *Candidate {
	if cv == nil || !cv.IsPathogenic() {
		return notApplicable(CodePP5, "no reputable pathogenic assertion")
	}
	return met(CodePP5, StrengthSupporting, "reputable assertion used only because raw evidence is unavailable",
		map[string]bool{"reputable_pathogenic_assertion": true, "independent_evidence_unavailable": true},
		"clinvar:"+cv.Version)
}

func deriveBP6(cv *annotation.ClinVar) *Candidate {
	if cv == nil || !cv.IsBenign() {
		return notApplicable(CodeBP6, "no reputable benign assertion")
	}
	return met(CodeBP6, StrengthSupporting, "reputable assertion used only because raw evidence is unavailable",
		map[string]bool{"reputable_benign_assertion": true, "independent_evidence_unavailable": true},
		"clinvar:"+cv.Version)
}

// derivePVS1 implements only the conservative branches that the inputs can
// actually establish. Anything else stays insufficient_data.
func derivePVS1(v SNVVariantInput, a *annotation.Set, ctx ClinicalContextInput, opts DeriveOptions) *Candidate {
	consequence := strings.ToLower(strings.TrimSpace(v.Consequence))
	nullVariant := strings.Contains(consequence, "stop_gained") || strings.Contains(consequence, "frameshift") ||
		strings.Contains(consequence, "nonsense") || strings.Contains(consequence, "start_lost") ||
		strings.Contains(consequence, "splice_donor") || strings.Contains(consequence, "splice_acceptor")
	if !nullVariant {
		return notApplicable(CodePVS1, "variant is not a null variant")
	}
	var missing []string
	if !opts.PVS1MechanismConfirmed {
		missing = append(missing, "lof_disease_mechanism")
	}
	if !opts.TranscriptRelevant {
		missing = append(missing, "transcript_relevant")
	}
	if strings.TrimSpace(ctx.Inheritance) == "" {
		missing = append(missing, "moi_consistent")
	}
	if len(missing) > 0 {
		return insufficient(CodePVS1, "PVS1 prerequisites are not attested", missing...)
	}
	if a != nil && a.GenCC != nil && a.GenCC.RefutedOrDisputed() {
		return &Candidate{Code: CodePVS1, Direction: DirectionPathogenic, Status: CriterionStatusConflicting, Strength: StrengthVeryStrong, Reason: "GenCC refutes or disputes the gene-disease assertion"}
	}
	strength := StrengthVeryStrong
	reason := "null variant with confirmed LOF mechanism and consistent inheritance"
	if isSpliceConsequence(consequence) {
		// Splice PVS1 without RNA data is capped at Strong until the decision tree
		// can establish NMD escape or functional impact.
		strength = StrengthStrong
		reason = "canonical splice variant; PVS1 strength capped at Strong pending NMD assessment"
	}
	facts := map[string]bool{"null_variant": true, "lof_disease_mechanism": true, "transcript_relevant": true, "nmd_or_functional_impact": strength == StrengthVeryStrong, "moi_consistent": true}
	return met(CodePVS1, strength, reason, facts)
}

func isSpliceConsequence(consequence string) bool {
	return strings.Contains(consequence, "splice_donor") || strings.Contains(consequence, "splice_acceptor") || strings.Contains(consequence, "splice")
}

// transcriptMatches treats an unspecified transcript as matching only when the
// caller did not pin one; a mismatch is a hard stop.
func transcriptMatches(annotationTranscript, variantTranscript string) bool {
	if annotationTranscript == "" || variantTranscript == "" {
		return annotationTranscript == variantTranscript
	}
	left := strings.SplitN(annotationTranscript, ".", 2)[0]
	right := strings.SplitN(variantTranscript, ".", 2)[0]
	return strings.EqualFold(left, right)
}

// Applied converts met candidates into the evaluator's criterion list.
func (d Derivation) Applied() []AppliedCriterion {
	var out []AppliedCriterion
	for _, c := range d.Candidates {
		if c.Status != CriterionStatusMet {
			continue
		}
		out = append(out, AppliedCriterion{Code: c.Code, EvaluatedStrength: c.Strength, Direction: c.Direction, Status: CriterionStatusMet, Rationale: c.Reason, SourceIDs: c.SourceIDs, Facts: c.Facts})
	}
	return out
}

// Gaps collects every missing prerequisite and non-met reason for the caller.
func (d Derivation) GapList() []string {
	var out []string
	for _, c := range d.Candidates {
		if c.Status == CriterionStatusMet {
			continue
		}
		for _, m := range c.MissingInformation {
			out = append(out, fmt.Sprintf("%s: %s", c.Code, m))
		}
	}
	sort.Strings(out)
	return out
}
