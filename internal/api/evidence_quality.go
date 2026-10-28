package api

import (
	"encoding/json"
	"github.com/pzweuj/sepilens/internal/assessment"
	"github.com/pzweuj/sepilens/internal/evidence"
	"reflect"
	"sort"
)

// Audit independently established support; model-written facts cannot verify themselves.
func evidenceQuality(base assessment.Result, p assessmentProposal, sources []evidence.Source) map[string]any {
	byID := map[string]evidence.Source{}
	for _, s := range sources {
		byID[s.ID] = s
	}
	prior := map[string]assessment.Criterion{}
	for _, c := range base.Criteria {
		prior[c.Code] = c
	}
	bindings := []map[string]any{}
	supported := 0
	counts := map[string]int{}
	for _, c := range p.Criteria {
		counts[c.Code]++
	}
	for _, c := range p.Criteria {
		refs := []map[string]any{}
		material := len(c.SourceIDs) > 0
		for _, id := range c.SourceIDs {
			s, exists := byID[id]
			valid := exists && s.IdentifierValid && s.RecordExists && s.ObjectMatched && s.MaterialVerified && s.ValidationStatus == evidence.ValidationVerified
			material = material && valid
			refs = append(refs, map[string]any{"source_id": id, "record_exists": exists && s.RecordExists, "object_matched": exists && s.ObjectMatched, "material_supported": valid})
		}
		b, exists := prior[c.Code]
		applicable := assessment.ValidCode(base.EngineID, c.Code) && counts[c.Code] == 1 && exists && b.Status == "met" && c.Status == "met" && material && c.Strength == b.Strength && reflect.DeepEqual(c.Score, b.Score) && reflect.DeepEqual(c.Points, b.Points)
		for _, id := range c.SourceIDs {
			found := false
			for _, priorID := range b.SourceIDs {
				found = found || id == priorID
			}
			applicable = applicable && found
		}
		for name, value := range b.Facts {
			if value {
				applicable = applicable && c.Facts[name]
			}
		}
		facts := []string{}
		for name, value := range c.Facts {
			if value {
				facts = append(facts, name)
				applicable = applicable && b.Facts[name]
			}
		}
		sort.Strings(facts)
		state := "pending_review"
		if c.Status == "insufficient_data" {
			state = "insufficient_data"
		}
		if c.Status == "conflicting" {
			state = "conflicting"
		}
		if applicable {
			state = "supported"
			supported++
		}
		bindings = append(bindings, map[string]any{"code": c.Code, "status": state, "facts": facts, "sources": refs, "applicability_supported": applicable})
	}
	return map[string]any{"version": "evidence-quality-v1", "criteria": bindings, "supported_count": supported, "proposal_count": len(p.Criteria)}
}

func modelFactPack(r assessment.Request, base assessment.Result, sources []evidence.Source) map[string]any {
	var result map[string]any
	raw, _ := json.Marshal(assessment.BuildImportedFactPack(r))
	_ = json.Unmarshal(raw, &result)
	byID := map[string]evidence.Source{}
	for _, source := range sources {
		byID[source.ID] = source
	}
	established := []map[string]any{}
	for _, criterion := range base.Criteria {
		supported := criterion.Status == "met" && len(criterion.SourceIDs) > 0 && assessment.ValidCode(r.EngineID, criterion.Code)
		versions := []map[string]any{}
		for _, id := range criterion.SourceIDs {
			source, exists := byID[id]
			supported = supported && exists && source.IdentifierValid && source.RecordExists && source.ObjectMatched && source.MaterialVerified && source.ValidationStatus == evidence.ValidationVerified
			version := source.DatasetVersion
			if version == "" {
				version = "unversioned"
			}
			versions = append(versions, map[string]any{"id": id, "version": version})
		}
		if !supported {
			continue
		}
		names := []string{}
		for name, value := range criterion.Facts {
			if value {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		for _, name := range names {
			established = append(established, map[string]any{"field": "criterion." + criterion.Code + "." + name, "code": criterion.Code, "value": true, "status": "supported", "source_ids": criterion.SourceIDs, "source_versions": versions, "path": "/rule_engine_result/criteria/" + criterion.Code + "/facts/" + name})
		}
	}
	result["established_facts"] = established
	return result
}
