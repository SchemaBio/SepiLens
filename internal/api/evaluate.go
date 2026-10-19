package api

import (
	"encoding/json"
	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
	"github.com/pzweuj/sepilens/internal/rules/cnvgain2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvloss2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvrubric"
	"github.com/pzweuj/sepilens/internal/variant/cnv"
	"strconv"
	"strings"
)

func (s *Server) evaluate(in envelope, sources []evidence.Source, items []evidence.EvidenceItem) any {
	verified := map[string]bool{}
	for _, x := range sources {
		verified[x.ID] = x.ValidationStatus == evidence.ValidationVerified && x.IdentifierValid && x.RecordExists && x.ObjectMatched && x.MaterialVerified
	}
	var selections []cnvrubric.Selection
	var criteria []acmg2015.AppliedCriterion
	for _, it := range items {
		ok := it.Status == evidence.StatusMet && len(it.SourceIDs) > 0
		for _, id := range it.SourceIDs {
			ok = ok && verified[id]
		}
		if !ok {
			continue
		}
		n, e := parseScore(it.SuggestedScore)
		if e == nil {
			selections = append(selections, cnvrubric.Selection{Code: it.RuleCode, Score: n, SourceIDs: it.SourceIDs, CaseIDs: it.CaseIDs, Facts: it.Facts, Verified: true})
		}
		def, exists := acmg2015.CanonicalCriteria[acmg2015.CriterionCode(it.RuleCode)]
		if exists {
			criteria = append(criteria, acmg2015.AppliedCriterion{Code: def.Code, EvaluatedStrength: acmg2015.Strength(it.SuggestedScore), Direction: def.Direction, Status: acmg2015.CriterionStatusMet, Rationale: it.Reason, SourceIDs: it.SourceIDs, Facts: it.Facts})
		}
	}
	if in.SNV != nil {
		r := in.SNV
		var v acmg2015.SNVVariantInput
		var c acmg2015.ClinicalContextInput
		b, _ := json.Marshal(r.Variant)
		_ = json.Unmarshal(b, &v)
		b, _ = json.Marshal(r.ClinicalContext)
		_ = json.Unmarshal(b, &c)
		if r.Annotations != nil {
			// Annotations supply the criterion facts; caller evidence items are not
			// mixed in, because a caller cannot pre-satisfy a derived criterion.
			derived := acmg2015.Derive(r.Annotations, v, c, deriveOptions(r))
			if p := r.phenotypeResult; p != nil {
				derived.Candidates = append(derived.Candidates, acmg2015.Candidate{Code: acmg2015.CodePP4, Direction: acmg2015.DirectionPathogenic, Status: acmg2015.CriterionStatus(p.PP4.Status), Strength: acmg2015.StrengthSupporting, Reason: "versioned phenotype score and ten PP4 gates", Facts: map[string]bool{"highly_specific_phenotype": p.PP4.Status == "met", "single_genetic_etiology": p.PP4.Gates["single_cause"]}, MissingInformation: p.PP4.Missing, SourceIDs: []string{"phenotype:" + p.Match.SpecVersion}})
			}
			return s.evaluateDerivation(r, derived, annotationSources(r, derived))
		}
		return s.acmgEvaluator.EvaluateEvidence(v, c, criteria, sources)
	}
	if in.CNV != nil {
		r := in.CNV
		if s.cnvAnnotator == nil {
			return map[string]string{"classification": "insufficient_data"}
		}
		a := s.cnvAnnotator.Annotate(cnv.CNVInput{Assembly: r.CNV.Assembly, Chrom: r.CNV.Chrom, Start: r.CNV.Start, End: r.CNV.End, Type: r.CNV.Type, CopyNumber: r.CNV.CopyNumber, BreakpointPrecision: r.CNV.BreakpointPrecision, Mosaicism: r.CNV.Mosaicism, Orientation: r.CNV.Orientation})
		if r.EngineID == "cnv_loss_2020" {
			return s.cnvLossEngine.Evaluate(cnvloss2020.LossEvaluationInput{CNV: a, Evidence: selections})
		}
		return s.cnvGainEngine.Evaluate(cnvgain2020.GainEvaluationInput{CNV: a, Evidence: selections})
	}
	return nil
}

// DerivedResult keeps the deterministic evaluation and its annotation provenance
// together, so callers can audit every criterion against its source record.
type DerivedResult struct {
	Phenotype *PhenotypeAssessment `json:"phenotype,omitempty"`
	acmg2015.EvaluationOutput
	Derivation        acmg2015.Derivation         `json:"derivation"`
	CombinationMethod string                      `json:"combination_method"`
	Points            *acmg2015.PointsCombination `json:"points,omitempty"`
}

func deriveOptions(r *SNVEvaluationRequest) acmg2015.DeriveOptions {
	opts := acmg2015.DeriveOptions{AllowPP5BP6: r.Options.AllowPP5BP6}
	// PVS1 attestations are supplied as explicit facts; they are never inferred
	// from GenCC validity or from the gene name.
	return opts
}

func combinationMethod(opts EvaluationOptions) string {
	if opts.CombinationMethod == "svi_points" {
		return "svi_points"
	}
	return "acmg_2015_table"
}

func pointsFor(opts EvaluationOptions, d acmg2015.Derivation) *acmg2015.PointsCombination {
	if combinationMethod(opts) != "svi_points" {
		return nil
	}
	points := acmg2015.CombinePoints(d.Applied())
	return &points
}

// annotationSources exposes the annotation versions as verification sources, so
// the evaluator's source gate has something auditable to check. Caller-supplied
// annotations stay unverified: they can be evaluated but not settled or adopted.
func annotationSources(r *SNVEvaluationRequest, d acmg2015.Derivation) []evidence.Source {
	if r.Annotations == nil {
		return nil
	}
	trusted := r.snapshotResolved
	var out []evidence.Source
	seen := map[string]bool{}
	for _, c := range d.Candidates {
		if c.Status != acmg2015.CriterionStatusMet {
			continue
		}
		for _, id := range c.SourceIDs {
			if seen[id] {
				continue
			}
			seen[id] = true
			version := strings.SplitN(id, ":", 2)
			datasetVersion := ""
			if len(version) == 2 {
				datasetVersion = version[1]
			}
			src := evidence.Source{ID: id, Type: evidence.SourceTypeCuratedSnapshot, Title: id, DatasetVersion: datasetVersion, IdentifierValid: true, RecordExists: true}
			if (trusted && usableAnnotationSource(r.Annotations, id)) || (strings.HasPrefix(id, "phenotype:") && r.phenotypeResult != nil && r.phenotypeResult.PP4.Status == "met") {
				src.ObjectMatched = true
				src.MaterialVerified = true
				src.ValidationStatus = evidence.ValidationVerified
			} else {
				src.RecordExists = false
				src.ValidationStatus = evidence.ValidationUnverified
			}
			out = append(out, src)
		}
	}
	return out
}

func parseScore(v string) (int64, error) {
	negative := strings.HasPrefix(v, "-")
	v = strings.TrimPrefix(v, "-")
	p := strings.Split(v, ".")
	if len(p) > 2 || len(p) == 0 {
		return 0, strconv.ErrSyntax
	}
	whole, e := strconv.ParseInt(p[0], 10, 32)
	if e != nil || whole < 0 || whole > 10 {
		return 0, strconv.ErrSyntax
	}
	fraction := int64(0)
	if len(p) == 2 {
		if len(p[1]) == 0 || len(p[1]) > 2 {
			return 0, strconv.ErrSyntax
		}
		f := p[1]
		if len(f) == 1 {
			f += "0"
		}
		fraction, e = strconv.ParseInt(f, 10, 16)
		if e != nil || fraction < 0 || fraction > 99 {
			return 0, strconv.ErrSyntax
		}
	}
	n := whole*100 + fraction
	if negative {
		n = -n
	}
	return n, nil
}
