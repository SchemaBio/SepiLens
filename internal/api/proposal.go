package api

import (
	"encoding/json"
	"errors"
	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
	"github.com/pzweuj/sepilens/internal/rules/cnvgain2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvloss2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvrubric"
	"github.com/pzweuj/sepilens/internal/workflow"
	"sort"
)

func proposalSchema(engine string, sources []evidence.Source) map[string]any {
	var ids, codes, facts []any
	for _, src := range sources {
		ids = append(ids, src.ID)
	}
	knownFacts := map[string]bool{"evidence_independent": true}
	if engine == "acmg_amp_2015" {
		for code := range acmg2015.CanonicalCriteria {
			codes = append(codes, string(code))
			for _, f := range acmg2015.RequiredFacts[code] {
				knownFacts[f] = true
			}
		}
	} else if engine == "svc_v4" {
		// The reference methods are analyst-coded. A model may organize sources but
		// cannot invent a draft assessment or borrow ACMG/CNV criteria.
		codes = []any{"unassigned"}
		knownFacts = map[string]bool{}
	} else {
		rules := cnvloss2020.Rules
		if engine == "cnv_gain_2020" {
			rules = cnvgain2020.Rules
		}
		for _, d := range cnvrubric.Parse(rules) {
			codes = append(codes, d.Code)
			for _, f := range d.Facts {
				knownFacts[f] = true
			}
		}
	}
	for f := range knownFacts {
		facts = append(facts, f)
	}
	sort.Slice(codes, func(i, j int) bool { return codes[i].(string) < codes[j].(string) })
	sort.Slice(facts, func(i, j int) bool { return facts[i].(string) < facts[j].(string) })
	max := 15.0
	if len(ids) == 0 {
		ids = []any{"unavailable"}
		max = 0
	}
	array := func(values []any) map[string]any {
		if len(values) == 0 {
			return map[string]any{"type": "array", "maxItems": 0.0, "items": map[string]any{"type": "string"}}
		}
		return map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": values}}
	}
	s := map[string]any{"type": "object", "properties": map[string]any{"summary": map[string]any{"type": "string", "maxLength": 4000.0}, "missing_information": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "evidence": map[string]any{"type": "array", "maxItems": max, "items": map[string]any{"type": "object", "properties": map[string]any{"rule_code": map[string]any{"type": "string", "enum": codes}, "suggested_score": map[string]any{"type": "string"}, "reason": map[string]any{"type": "string", "maxLength": 2000.0}, "source_ids": array(ids), "facts_confirmed": array(facts)}, "required": []any{"rule_code", "suggested_score", "reason", "source_ids", "facts_confirmed"}, "additionalProperties": false}}}, "required": []any{"summary", "missing_information", "evidence"}, "additionalProperties": false}
	// Normalize schema number/list representations to the validator's JSON types.
	b, _ := json.Marshal(s)
	_ = json.Unmarshal(b, &s)
	return s
}
func parseProposal(raw, engine string, sources []evidence.Source) (any, []evidence.EvidenceItem, error) {
	var p struct {
		Summary  string   `json:"summary"`
		Missing  []string `json:"missing_information"`
		Evidence []struct {
			Code    string   `json:"rule_code"`
			Score   string   `json:"suggested_score"`
			Reason  string   `json:"reason"`
			Sources []string `json:"source_ids"`
			Facts   []string `json:"facts_confirmed"`
		} `json:"evidence"`
	}
	if json.Unmarshal([]byte(raw), &p) != nil {
		return nil, nil, errors.New("invalid proposal")
	}
	known := map[string]bool{}
	for _, s := range sources {
		known[s.ID] = true
	}
	var items []evidence.EvidenceItem
	for _, it := range p.Evidence {
		if len(it.Sources) == 0 {
			return nil, nil, errors.New("proposal citation required")
		}
		for _, id := range it.Sources {
			if !known[id] {
				return nil, nil, errors.New("proposal cites unknown source")
			}
		}
		facts := map[string]bool{}
		for _, f := range it.Facts {
			facts[f] = true
		}
		items = append(items, evidence.EvidenceItem{ID: workflow.ID("evi-"), EngineID: engine, RuleCode: it.Code, SuggestedScore: it.Score, Reason: it.Reason, SourceIDs: it.Sources, Facts: facts, Status: evidence.StatusProposedMet, SourceValidation: "unverified", ReviewRequired: true})
	}
	return map[string]any{"summary": p.Summary, "missing_information": p.Missing}, items, nil
}
