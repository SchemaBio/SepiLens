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
