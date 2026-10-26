package api

import (
	"github.com/pzweuj/sepilens/internal/agent"
	"github.com/pzweuj/sepilens/internal/assessment"
	"github.com/pzweuj/sepilens/internal/evidence"
	"strings"
)

type svcContextField struct {
	Value     string   `json:"value"`
	Reason    string   `json:"reason"`
	SourceIDs []string `json:"source_ids"`
}
type svcContextProposal struct {
	Disease             svcContextField   `json:"disease"`
	MOI                 svcContextField   `json:"moi"`
	GeneDiseaseValidity svcContextField   `json:"geneDiseaseValidity"`
	Workflow            svcContextField   `json:"workflow"`
	Sources             []evidence.Source `json:"sources,omitempty"`
	RetrievalStatus     string            `json:"retrieval_status,omitempty"`
}

func svcContextSchema() map[string]any {
	field := func(values []string) map[string]any {
		value := map[string]any{"type": "string", "maxLength": 500}
		if values != nil {
			value["enum"] = values
		}
		return map[string]any{"type": "object", "additionalProperties": false, "required": []any{"value", "reason", "source_ids"}, "properties": map[string]any{"value": value, "reason": map[string]any{"type": "string", "maxLength": 2000}, "source_ids": map[string]any{"type": "array", "maxItems": 20, "items": map[string]any{"type": "string"}}}}
	}
	return map[string]any{"type": "object", "additionalProperties": false, "required": []any{"disease", "moi", "geneDiseaseValidity", "workflow"}, "properties": map[string]any{
		"disease": field(nil), "moi": field([]string{"", "AD", "AR", "XLD", "XLR", "SD"}), "geneDiseaseValidity": field([]string{"", "DEFINITIVE", "STRONG", "MODERATE", "LIMITED", "DISPUTED", "REFUTED", "NOT_CLASSIFIED"}), "workflow": field([]string{"", "missense", "nonsense", "frameshift", "canonical_splice", "intronic_synonymous", "exon_deletion", "exon_duplication", "start_lost", "stop_lost"}),
	}}
}
func validateSVCContext(p *svcContextProposal, r assessment.Request, sources []evidence.Source, aliases map[string]string) error {
	if r.EngineID != "svc_v4" {
		return agent.ErrOutputInvalid
	}
	known := map[string]bool{}
	for _, s := range sources {
		known[s.ID] = true
	}
	used := map[string]bool{}
	for _, field := range []*svcContextField{&p.Disease, &p.MOI, &p.GeneDiseaseValidity, &p.Workflow} {
		field.Value = strings.TrimSpace(field.Value)
		if field.Value != "" && (strings.TrimSpace(field.Reason) == "" || len(field.SourceIDs) == 0) {
			field.Value = ""
			field.Reason = "No traceable supporting evidence was supplied."
			field.SourceIDs = []string{}
		}
		for i, id := range field.SourceIDs {
			if id == "annotation" && r.Package.AIContext != nil {
				continue
			}
			if original, ok := aliases[id]; ok {
				id = original
				field.SourceIDs[i] = id
			}
			if !known[id] {
				return agent.ErrOutputInvalid
			}
			used[id] = true
		}
	}
	p.Sources = nil
	for _, s := range sources {
		if used[s.ID] {
			p.Sources = append(p.Sources, s)
		}
	}
	return nil
}
