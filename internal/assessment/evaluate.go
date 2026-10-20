package assessment

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/pzweuj/sepilens/internal/annotation"
	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/hpo"
	"github.com/pzweuj/sepilens/internal/phenotype"
	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
	"github.com/pzweuj/sepilens/internal/rules/cnvgain2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvloss2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvrubric"
	"github.com/pzweuj/sepilens/internal/rules/svcv4"
	"github.com/pzweuj/sepilens/internal/variant/cnv"
)

func Evaluate(r Request, sources []evidence.Source) Result {
	out := Result{EngineID: r.EngineID, Status: "insufficient_data", Classification: "insufficient_data", Criteria: []Criterion{}, Missing: []string{}}
	if r.EngineID == "svc_v4" {
		if r.Package.Draft == nil || !clinical(r.Package.DraftSourceIDs, r, sources) {
			out.Missing = append(out.Missing, "versioned draft inputs and trusted sources required")
			return out
		}
		d, err := svcv4.Evaluate(r.Package.Draft)
		if err != nil {
			out.Missing = append(out.Missing, "draft input invalid")
			return out
		}
		out.Experimental = d
		for _, f := range d.Families {
			status := "insufficient_data"
			if f.Total != nil {
				status = "met"
			}
			out.Criteria = append(out.Criteria, Criterion{Code: f.ParentCode, Points: f.Total, Status: status, SourceIDs: r.Package.DraftSourceIDs, Missing: f.Missing})
		}
		out.Status = "experimental"
		return out
	}
	if r.Variant != nil {
		a := &annotation.Set{}
		if r.Package.Annotations != nil {
			b, _ := json.Marshal(r.Package.Annotations)
			_ = json.Unmarshal(b, a)
		}
		// Ignore payload trust labels. Only policy-accepted, matching records derive facts.
		filter := func(kind, version string) bool {
			for _, s := range r.Package.Sources {
				if s.Kind == kind && s.Version == version && trusted([]string{s.ID}, sources) {
					return true
				}
			}
			return false
		}
		if a.ClinVar != nil && !filter("clinvar", a.ClinVar.Version) {
			a.ClinVar = nil
		}
		if a.GnomAD != nil && !filter("gnomad", a.GnomAD.Version) {
			a.GnomAD = nil
		}
		if a.AlphaMissense != nil && !filter("alphamissense", a.AlphaMissense.Version) {
			a.AlphaMissense = nil
		}
		if a.Pangolin != nil && !filter("pangolin", a.Pangolin.Version) {
			a.Pangolin = nil
		}
		if a.GenCC != nil && !filter("gencc", a.GenCC.Version) {
			a.GenCC = nil
		}
		opts := acmg2015.DeriveOptions{}
		if m := r.Package.Mechanism; m != nil && clinical(m.SourceIDs, r, sources) && m.Transcript == r.Variant.Transcript && m.NMDApplicable {
			opts.PVS1MechanismConfirmed = m.LOFEstablished
			opts.TranscriptRelevant = m.TranscriptRelevant
		}
		d := acmg2015.Derive(a, *r.Variant, r.Context, opts)
		for _, c := range d.Candidates {
			var ids []string
			for _, id := range c.SourceIDs {
				kind := strings.SplitN(id, ":", 2)[0]
				ids = append(ids, kindIDs(r, kind)...)
			}
			if c.Code == "PVS1" && r.Package.Mechanism != nil {
				ids = append(ids, r.Package.Mechanism.SourceIDs...)
			}
			if len(ids) == 0 {
				ids = kindIDs(r, "consequence")
			}
			out.Criteria = append(out.Criteria, Criterion{Code: string(c.Code), Status: string(c.Status), Strength: string(c.Strength), SourceIDs: ids, Facts: c.Facts, Reason: c.Reason, Missing: c.MissingInformation})
		}
		if p := r.Package.Phenotype; p != nil {
			match, pp := matchPhenotype(r, sources)
			out.Phenotype = map[string]any{"match": match, "pp4": pp}
			ids := append(append([]string{}, p.SourceIDs...), p.ClinicalSourceIDs...)
			ids = append(ids, kindIDs(r, "gencc")...)
			out.Criteria = append(out.Criteria, Criterion{Code: "PP4", Status: pp.Status, Strength: "Supporting", SourceIDs: ids, Facts: map[string]bool{"highly_specific_phenotype": pp.Status == "met", "single_genetic_etiology": pp.Gates["single_cause"]}, Missing: pp.Missing})
		}
		out.Missing = append(out.Missing, d.Gaps...)
		out.Missing = append(out.Missing, d.Conflicts...)
	} else {
		out.Criteria = deriveCNV(r, sources)
	}
	// Analyst-coded clinical material is separate from public annotations.
	for _, c := range r.Package.Criteria {
		if !clinical(c.SourceIDs, r, sources) {
			c.Status = "insufficient_data"
			c.Missing = append(c.Missing, "independently authorized clinical material required")
		}
		if c.Code == "PP4" {
			continue
		} // PP4 is always computed through all ten gates.
		if c.Code == "PVS1" {
			m := r.Package.Mechanism
			if m == nil || !clinical(m.SourceIDs, r, sources) || !m.LOFEstablished || !m.TranscriptRelevant || !m.NMDApplicable || m.Transcript != r.Variant.Transcript {
				c.Status = "insufficient_data"
			}
		}
		found := false
		for n, b := range out.Criteria {
			if b.Code == c.Code {
				if c.Status == "met" {
					out.Criteria[n] = c
				}
				found = true
				break
			}
		}
		if !found {
			out.Criteria = append(out.Criteria, c)
		}
	}
	return Recompute(r, out, sources)
}

func Recompute(r Request, out Result, sources []evidence.Source) Result {
	if r.EngineID == "svc_v4" {
		return Evaluate(r, sources)
	}
	sort.SliceStable(out.Criteria, func(i, j int) bool { return out.Criteria[i].Code < out.Criteria[j].Code })
	for n := range out.Criteria {
		c := &out.Criteria[n]
		if c.Code == "PVS1" && c.Status == "met" {
			m := r.Package.Mechanism
			if m == nil || !clinical(m.SourceIDs, r, sources) || !m.LOFEstablished || !m.TranscriptRelevant || !m.NMDApplicable || r.Variant == nil || m.Transcript != r.Variant.Transcript {
				c.Status = "insufficient_data"
			}
		}
		if c.Status == "met" && !trusted(c.SourceIDs, sources) {
			c.Status = "insufficient_data"
			c.Missing = append(c.Missing, "trusted applicable sources required")
		}
		if c.Status == "insufficient_data" || c.Status == "conflicting" {
			out.Missing = append(out.Missing, c.Code+": applicable material required")
		}
	}
	if r.EngineID == "acmg_amp_2015" {
		var applied []acmg2015.AppliedCriterion
		for _, c := range out.Criteria {
			def := acmg2015.CanonicalCriteria[acmg2015.CriterionCode(c.Code)]
			applied = append(applied, acmg2015.AppliedCriterion{Code: def.Code, Direction: def.Direction, Status: acmg2015.CriterionStatus(c.Status), EvaluatedStrength: acmg2015.Strength(c.Strength), SourceIDs: c.SourceIDs, Facts: c.Facts, Rationale: c.Reason})
		}
		v := *r.Variant
		if a := r.Package.Annotations; a != nil && a.GnomAD != nil && trusted(kindIDs(r, "gnomad"), sources) {
			v.AlleleFrequency = a.GnomAD.PopMaxAF
			if v.AlleleFrequency == nil {
				v.AlleleFrequency = a.GnomAD.FAF95
			}
		}
		res := acmg2015.NewEvaluator(nil).EvaluateEvidence(v, r.Context, applied, sources)
		accepted := map[string]bool{}
		for _, c := range res.AppliedCriteria {
			if c.Status == acmg2015.CriterionStatusMet {
				accepted[string(c.Code)] = true
			}
		}
		for n := range out.Criteria {
			c := &out.Criteria[n]
			if c.Status == "met" && !accepted[c.Code] {
				c.Status = "insufficient_data"
				c.Missing = append(c.Missing, "rule applicability or strength prerequisites not satisfied")
			}
		}
		if r.Options.CombinationMethod == "svi_points" {
			res.Classification = acmg2015.CombinePoints(res.AppliedCriteria).Classification
		}
		for _, a := range r.Package.GeneAssertions {
			if a.DiseaseID == r.Context.DiseaseID && a.Gene == v.Gene && a.RefutedOrDisputed() && trusted(kindIDs(r, "gencc"), sources) {
				res.Classification = acmg2015.ClassVUS
				out.Missing = append(out.Missing, "refuted or disputed gene-disease assertion")
			}
		}
		out.Computation = res
		out.Classification = res.Classification
		out.Status = "calculated"
		if len(accepted) == 0 {
			out.Classification = "insufficient_data"
			out.Status = "insufficient_data"
		}
		return out
	}
	if r.CNV != nil {
		var selections []cnvrubric.Selection
		for _, c := range out.Criteria {
			if c.Status == "met" && c.Score != nil {
				d := r.Package.Dosage
				if strings.HasPrefix(c.Code, "3") && (d == nil || d.ProteinCodingGenes == nil || !trusted(d.GeneCountSourceIDs, sources)) {
					continue
				}
				if (c.Code == "1A" || c.Code == "1B") && (d == nil || d.FunctionalElements == nil || !trusted(d.FunctionalSourceIDs, sources)) {
					continue
				}
				selections = append(selections, cnvrubric.Selection{Code: c.Code, Score: *c.Score, SourceIDs: c.SourceIDs, CaseIDs: c.CaseIDs, Facts: c.Facts, Verified: trusted(c.SourceIDs, sources)})
			}
		}
		a := annotateCNV(r, sources)
		var res cnvrubric.Result
		if r.EngineID == "cnv_loss_2020" {
			res = cnvloss2020.NewEngine().Evaluate(cnvloss2020.LossEvaluationInput{CNV: a, Evidence: selections})
		} else {
			res = cnvgain2020.NewEngine().Evaluate(cnvgain2020.GainEvaluationInput{CNV: a, Evidence: selections})
		}
		out.SectionScores = map[string]int64{}
		for _, c := range res.Clauses {
			out.SectionScores[c.Section] += c.Score
		}
		out.Computation = res
		out.Classification = res.Classification
		out.Status = res.Status
		out.Missing = append(out.Missing, res.MissingInformation...)
	}
	return out
}

// ClinicalMaterial is the independent applicability basis available to an AI
// reconciliation. Public source labels do not authorize clinical assertions.
func ClinicalMaterial(r Request, c Criterion, sources []evidence.Source) bool {
	return clinical(c.SourceIDs, r, sources)
}

func annotateCNV(r Request, sources []evidence.Source) cnv.AnnotatedCNV {
	d := r.Package.Dosage
	if d == nil || !trusted(d.SourceIDs, sources) {
		return cnv.AnnotatedCNV{Input: r.CNV.CNVInput}
	}
	complete := d.Complete
	for _, id := range d.SourceIDs {
		for _, s := range r.Package.Sources {
			if s.ID == id {
				complete = complete && s.Complete
			}
		}
	}
	var assemblies []string
	if complete {
		assemblies = []string{r.CNV.Assembly}
	}
	a := cnv.NewSnapshotAnnotator(d.Regions, assemblies, HashPackage(r.Package)).Annotate(r.CNV.CNVInput)
	if d.ProteinCodingGenes != nil && trusted(d.GeneCountSourceIDs, sources) {
		a.GeneCount = *d.ProteinCodingGenes
	}
	if d.FunctionalElements != nil && trusted(d.FunctionalSourceIDs, sources) {
		a.ContainsFunctionalElements = *d.FunctionalElements
	}
	return a
}
func deriveCNV(r Request, sources []evidence.Source) []Criterion {
	a := annotateCNV(r, sources)
	d := r.Package.Dosage
	if d == nil {
		return []Criterion{{Code: "2A", Status: "insufficient_data", Missing: []string{"dosage reference required"}}}
	}
	var out []Criterion
	add := func(code string, n int64, fact string, ids []string) {
		out = append(out, Criterion{Code: code, Status: "met", Score: &n, SourceIDs: ids, Facts: map[string]bool{fact: true}})
	}
	if d.FunctionalElements != nil && trusted(d.FunctionalSourceIDs, sources) {
		if *d.FunctionalElements {
			add("1A", 0, "functional_elements", d.FunctionalSourceIDs)
		} else if a.CoverageComplete {
			n := int64(-60)
			if r.EngineID == "cnv_gain_2020" {
				n = -60
			}
			add("1B", n, "no_functional_elements", d.FunctionalSourceIDs)
		}
	}
	if r.EngineID == "cnv_loss_2020" && a.CompleteHIOverlap {
		add("2A", 100, "complete_hi_overlap", d.SourceIDs)
	} else if r.EngineID == "cnv_gain_2020" && a.CompleteTSOverlap {
		add("2A", 100, "complete_ts_overlap", d.SourceIDs)
	}
	if a.CoverageComplete && d.ProteinCodingGenes != nil && trusted(d.GeneCountSourceIDs, sources) {
		lo, hi := 25, 35
		if r.EngineID == "cnv_gain_2020" {
			lo, hi = 35, 50
		}
		code, n, f := "3A", int64(0), "gene_count_lower"
		if a.GeneCount >= hi {
			code, n, f = "3C", 90, "gene_count_upper"
		} else if a.GeneCount >= lo {
			code, n, f = "3B", 45, "gene_count_middle"
		}
		add(code, n, f, d.GeneCountSourceIDs)
	}
	if len(out) == 0 {
		out = append(out, Criterion{Code: "2A", Status: "insufficient_data", Missing: []string{"complete applicable dosage evidence required"}})
	}
	return out
}

func matchPhenotype(r Request, sources []evidence.Source) (phenotype.MatchResult, phenotype.PP4Result) {
	p := r.Package.Phenotype
	missing := func(reason string) (phenotype.MatchResult, phenotype.PP4Result) {
		m := phenotype.MatchResult{SpecVersion: phenotype.SpecVersion, Missing: []string{reason}}
		return m, phenotype.PP4(m, r.Context.DiseaseID, phenotype.PP4Context{})
	}
	if !trusted(p.SourceIDs, sources) || !p.OntologyComplete || p.Version == "" || p.OntologyVersion == "" {
		return missing("verified versioned ontology and reference material required")
	}
	g, err := hpo.NewGraph(p.Terms)
	if err != nil {
		return missing("invalid ontology graph")
	}
	d, err := phenotype.NewDataset(g, p.Associations, p.Version)
	if err != nil {
		return missing("incomplete phenotype material")
	}
	d.OntologyVersion = p.OntologyVersion
	// Candidate-subset frequencies must never substitute for population background.
	for id := range d.Background {
		if _, ok := p.Background[id]; !ok {
			return missing("explicit background frequencies required")
		}
	}
	d.Background = p.Background
	candidates := append([]string{r.Context.DiseaseID}, p.CandidateDiseases...)
	genes := map[string]bool{}
	pc := phenotype.PP4Context{InputsVerified: true, ClinicalVerified: clinical(p.ClinicalSourceIDs, r, sources) && p.Input.Source == "clinician_verified"}
	gencc := trusted(kindIDs(r, "gencc"), sources)
	for _, a := range r.Package.GeneAssertions {
		if !gencc {
			continue
		}
		if a.Gene == r.Variant.Gene && (a.GenCCSupports() || strings.EqualFold(a.Classification, "Moderate")) {
			candidates = append(candidates, a.DiseaseID)
		}
		if a.DiseaseID == r.Context.DiseaseID {
			if a.GenCCSupports() {
				genes[a.Gene] = true
			}
			if a.Gene == r.Variant.Gene {
				pc.RefutedOrDisputed = pc.RefutedOrDisputed || a.RefutedOrDisputed()
				pc.GenCCStrong = pc.GenCCStrong || a.GenCCSupports()
				pc.MOIConsistent = pc.MOIConsistent || (a.MOI == r.Context.Inheritance && a.GenCCSupports())
			}
		}
	}
	complete := p.GeneAssertionsComplete && gencc
	for _, s := range r.Package.Sources {
		if s.Kind == "gencc" {
			complete = complete && s.Complete
		}
	}
	pc.SingleCauseConfirmed = complete && len(genes) == 1 && genes[r.Variant.Gene]
	input := p.Input
	if !pc.ClinicalVerified {
		input.Source = "caller_supplied"
	}
	m, err := d.Match(input, candidates)
	if err != nil {
		return missing("unresolved phenotype or disease identifier")
	}
	if len(m.Missing) > 0 {
		pc.SingleCauseConfirmed = false
	}
	return m, phenotype.PP4(m, r.Context.DiseaseID, pc)
}
