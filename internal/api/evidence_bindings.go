package api

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/pzweuj/sepilens/internal/assessment"
	"github.com/pzweuj/sepilens/internal/evidence"
)

type modelEvidenceBinding struct {
	Fact     string `json:"fact"`
	FactID   string `json:"fact_id"`
	SourceID string `json:"source_id"`
	Excerpt  string `json:"excerpt"`
}
type checkedEvidenceBinding struct {
	modelEvidenceBinding
	Path             string `json:"path,omitempty"`
	Origin           string `json:"origin"`
	Status           string `json:"status"`
	Reason           string `json:"reason"`
	SourceTitle      string `json:"source_title,omitempty"`
	SourceURL        string `json:"source_url,omitempty"`
	SourceVersion    string `json:"source_version,omitempty"`
	RecordExists     bool   `json:"record_exists"`
	ObjectMatched    bool   `json:"object_matched"`
	MaterialVerified bool   `json:"material_verified"`
}
type bindingFact struct {
	Field     string   `json:"field"`
	Code      string   `json:"code"`
	Value     any      `json:"value"`
	Status    string   `json:"status"`
	Path      string   `json:"path"`
	SourceIDs []string `json:"source_ids"`
	Origin    string   `json:"origin"`
}

const evidenceBindingSystemPrompt = `For every met criterion bind each facts_confirmed entry to evidence_bindings using the exact same fact name. Each binding has fact, fact_id, source_id and excerpt strings. Use only fact_id values in the supplied fact_pack, or copy an exact nontrivial excerpt (at least 12 characters) from a supplied source snippet and cite that source's opaque ID. A binding uses either fact_id or excerpt, not both. Imported reported facts are context only and cannot establish clinical prerequisites. Established criterion facts are scoped to their exact engine, criterion and fact; never reuse another criterion's fact. For criteria with no named facts use an empty fact string and bind the criterion itself. Do not fabricate missing source content. Source existence, a gene mention, or locating a quote does not independently verify clinical applicability. If support is missing return insufficient_data with null score/points and an empty facts_confirmed list. An empty evidence_bindings list is permitted for unassessed/unmet clauses. Preserve valid supported clauses when correcting other clauses. Cite only supplied source IDs, and never invent case counts or verified family facts.`

func addEvidenceBindingSchema(schema map[string]any) {
	criteria := schema["properties"].(map[string]any)["criteria"].(map[string]any)
	item := criteria["items"].(map[string]any)
	item["required"] = append(item["required"].([]any), "evidence_bindings")
	item["properties"].(map[string]any)["evidence_bindings"] = map[string]any{"type": "array", "maxItems": 32, "items": map[string]any{"type": "object", "additionalProperties": false, "required": []any{"fact", "fact_id", "source_id", "excerpt"}, "properties": map[string]any{"fact": map[string]any{"type": "string", "maxLength": 128}, "fact_id": map[string]any{"type": "string", "maxLength": 256}, "source_id": map[string]any{"type": "string", "maxLength": 256}, "excerpt": map[string]any{"type": "string", "maxLength": 1200}}}}
}
func evidenceBindingCatalog(pack map[string]any) map[string]bindingFact {
	var wire struct {
		Facts       []bindingFact `json:"facts"`
		Established []bindingFact `json:"established_facts"`
		Structured  struct {
			Facts []bindingFact `json:"facts"`
		} `json:"structured_inputs"`
	}
	raw, _ := json.Marshal(pack)
	_ = json.Unmarshal(raw, &wire)
	out := map[string]bindingFact{}
	for _, fact := range wire.Facts {
		out[fact.Field] = fact
	}
	for _, fact := range wire.Established {
		out[fact.Field] = fact
	}
	for _, fact := range wire.Structured.Facts {
		out[fact.Field] = fact
	}
	return out
}

func bindingContains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
func verifiedBindingSource(source evidence.Source) bool {
	return source.IdentifierValid && source.RecordExists && source.ObjectMatched && source.MaterialVerified && source.ValidationStatus == evidence.ValidationVerified
}

// Locating an excerpt proves traceability only. Only independently established
// engine facts may mark a binding supported; no text-matching heuristic verifies
// a clinical proposition or a disease/variant association.
func checkEvidenceBindings(c assessmentChange, catalog map[string]bindingFact, sources []evidence.Source) ([]checkedEvidenceBinding, []string) {
	blocked := evidence.ConflictingSourceGroupIDs(sources)
	byID := map[string]evidence.Source{}
	for _, source := range sources {
		byID[source.ID] = source
	}
	checked := []checkedEvidenceBinding{}
	issues := []string{}
	seen := map[modelEvidenceBinding]bool{}
	addInvalid := func(binding modelEvidenceBinding, reason string) {
		checked = append(checked, checkedEvidenceBinding{modelEvidenceBinding: binding, Origin: "model", Status: "invalid", Reason: reason})
		if c.Status == "met" {
			issues = append(issues, reason)
		}
	}
	for _, binding := range c.Bindings {
		if seen[binding] {
			addInvalid(binding, "duplicate_binding")
			continue
		}
		seen[binding] = true
		if binding.Fact != "" && !c.Facts[binding.Fact] {
			addInvalid(binding, "claim_not_asserted")
			continue
		}
		if binding.FactID != "" {
			fact, exists := catalog[binding.FactID]
			if !exists {
				addInvalid(binding, "unknown_fact_id")
				continue
			}
			if binding.Excerpt != "" {
				addInvalid(binding, "ambiguous_binding")
				continue
			}
			if strings.HasPrefix(binding.FactID, "criterion.") {
				expected := "criterion." + c.Code
				if binding.Fact != "" {
					expected += "." + binding.Fact
				}
				if fact.Code != c.Code || binding.FactID != expected {
					addInvalid(binding, "fact_scope_mismatch")
					continue
				}
				sourceID := binding.SourceID
				if sourceID == "" {
					for _, id := range fact.SourceIDs {
						if bindingContains(c.SourceIDs, id) {
							sourceID = id
							break
						}
					}
				}
				source, exists := byID[sourceID]
				if fact.Status != "supported" || !bindingContains(fact.SourceIDs, sourceID) || !bindingContains(c.SourceIDs, sourceID) || !exists || !verifiedBindingSource(source) || blocked[sourceID] {
					addInvalid(binding, "structured_support_unavailable")
					continue
				}
				binding.SourceID = sourceID
				checked = append(checked, checkedEvidenceBinding{modelEvidenceBinding: binding, Path: fact.Path, Origin: "model", Status: "supported", Reason: "independent_structured_fact"})
			} else if fact.Origin == "structured_input" {
				if fact.Status != "reported" || fact.Value == nil {
					addInvalid(binding, "referenced_fact_unavailable")
					continue
				}
				if binding.SourceID != "" && (!bindingContains(fact.SourceIDs, binding.SourceID) || !bindingContains(c.SourceIDs, binding.SourceID) || blocked[binding.SourceID]) {
					addInvalid(binding, "request_fact_cannot_claim_source")
					continue
				}
				checked = append(checked, checkedEvidenceBinding{modelEvidenceBinding: binding, Path: fact.Path, Origin: "model", Status: "context_only", Reason: "structured_input_requires_rule_applicability"})
			} else {
				if binding.SourceID != "" {
					addInvalid(binding, "request_fact_cannot_claim_source")
					continue
				}
				if fact.Status != "reported" || fact.Value == nil {
					addInvalid(binding, "referenced_fact_unavailable")
					continue
				}
				checked = append(checked, checkedEvidenceBinding{modelEvidenceBinding: binding, Path: fact.Path, Origin: "model", Status: "context_only", Reason: "imported_context_only"})
			}
			continue
		}
		source, exists := byID[binding.SourceID]
		switch {
		case !exists:
			addInvalid(binding, "unknown_source")
		case blocked[binding.SourceID]:
			addInvalid(binding, "source_group_conflicting")
		case !bindingContains(c.SourceIDs, binding.SourceID):
			addInvalid(binding, "source_not_cited")
		case source.ValidationStatus == evidence.ValidationConflict || source.ValidationStatus == evidence.ValidationRejected:
			addInvalid(binding, "source_conflicting_or_rejected")
		case utf8.RuneCountInString(strings.TrimSpace(binding.Excerpt)) < 12 || !strings.Contains(source.Snippet, strings.TrimSpace(binding.Excerpt)):
			addInvalid(binding, "excerpt_not_located")
		default:
			checked = append(checked, checkedEvidenceBinding{modelEvidenceBinding: binding, Origin: "model", Status: "pending_review", Reason: "excerpt_requires_review"})
		}
	}
	if c.Status == "met" {
		claims := []string{}
		for fact, value := range c.Facts {
			if value {
				claims = append(claims, fact)
			}
		}
		sort.Strings(claims)
		if len(claims) == 0 {
			claims = []string{""}
		}
		for _, claim := range claims {
			hasSupport := false
			for _, item := range checked {
				if item.Fact == claim && (item.Status == "supported" || item.Status == "pending_review") {
					hasSupport = true
				}
			}
			// Recover an explicit, independently established baseline proof without
			// asking a paid model to repeat it. Its provenance remains rule_engine.
			id := "criterion." + c.Code
			if claim != "" {
				id += "." + claim
			}
			fact, exists := catalog[id]
			if exists && fact.Code == c.Code && fact.Status == "supported" {
				for _, sourceID := range fact.SourceIDs {
					source, present := byID[sourceID]
					if present && verifiedBindingSource(source) && !blocked[sourceID] && bindingContains(c.SourceIDs, sourceID) {
						already := false
						for _, item := range checked {
							already = already || item.Fact == claim && item.Status == "supported"
						}
						if !already {
							checked = append(checked, checkedEvidenceBinding{modelEvidenceBinding: modelEvidenceBinding{Fact: claim, FactID: id, SourceID: sourceID}, Path: fact.Path, Origin: "rule_engine", Status: "supported", Reason: "independent_structured_fact"})
						}
						hasSupport = true
						break
					}
				}
			}
			if !hasSupport {
				issues = append(issues, "missing_independent_or_excerpt_binding")
			}
		}
	}
	for index := range checked {
		if source, exists := byID[checked[index].SourceID]; exists {
			checked[index].SourceTitle = source.Title
			checked[index].SourceURL = source.URL
			checked[index].SourceVersion = source.DatasetVersion
			checked[index].RecordExists = source.RecordExists
			checked[index].ObjectMatched = source.ObjectMatched
			checked[index].MaterialVerified = source.MaterialVerified
		}
	}
	return checked, issues
}

// Review reasons contain fixed enums and array offsets only, never excerpts,
// patient information, provider errors, source IDs or model-written paths.
func evidenceBindingReview(raw json.RawMessage, pack map[string]any, sources []evidence.Source, reverse map[string]string) []string {
	var wire modelProposal
	if json.Unmarshal(raw, &wire) != nil {
		return []string{"$: evidence_binding_decode_failed"}
	}
	catalog := evidenceBindingCatalog(pack)
	issues := []string{}
	for index, item := range wire.Criteria {
		c := assessmentChange{Criterion: assessment.Criterion{Code: item.Code, Status: item.Status, Facts: map[string]bool{}}, Bindings: append([]modelEvidenceBinding(nil), item.Bindings...)}
		for _, fact := range item.Facts {
			c.Facts[fact] = true
		}
		for _, id := range item.SourceIDs {
			if original, ok := reverse[id]; ok {
				id = original
			}
			c.SourceIDs = append(c.SourceIDs, id)
		}
		for i := range c.Bindings {
			if original, ok := reverse[c.Bindings[i].SourceID]; ok {
				c.Bindings[i].SourceID = original
			}
		}
		_, found := checkEvidenceBindings(c, catalog, sources)
		for _, reason := range found {
			if len(issues) >= 24 {
				return issues
			}
			issues = append(issues, fmt.Sprintf("criteria[%d].evidence_bindings: %s", index, reason))
		}
	}
	return issues
}

func projectEvidenceBindings(p assessmentProposal, pack map[string]any, sources []evidence.Source) (assessmentProposal, []map[string]any, []string) {
	catalog := evidenceBindingCatalog(pack)
	reports := []map[string]any{}
	warnings := []string{}
	for index, c := range p.Criteria {
		checked, issues := checkEvidenceBindings(c, catalog, sources)
		reports = append(reports, map[string]any{"code": c.Code, "bindings": checked, "issues": issues})
		if len(issues) > 0 && c.Status == "met" {
			c.Status = "insufficient_data"
			c.Score = nil
			c.Points = nil
			c.Facts = map[string]bool{}
			c.Change = "downgraded"
			p.Criteria[index] = c
			warnings = append(warnings, fmt.Sprintf("criteria[%d].evidence_bindings: unsupported_claim_defaulted", index))
		}
	}
	if len(warnings) > 0 {
		p.Classification = "insufficient_data"
		p.ClinicalSummary = "部分条款的证据绑定未通过核查，已降为证据不足；原始建议仅供复核。\n" + p.ClinicalSummary
	}
	return p, reports, warnings
}
