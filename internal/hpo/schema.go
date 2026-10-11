package hpo

import (
	"encoding/json"
)

func preprocessSchema() map[string]any {
	var s map[string]any
	_ = json.Unmarshal([]byte(`{"type":"object","properties":{"symptoms":{"type":"array","items":{"type":"string"}},"negated_symptoms":{"type":"array","items":{"type":"string"}},"medical_history":{"type":"array","items":{"type":"string"}},"diagnosis":{"type":"array","items":{"type":"string"}},"family_history":{"type":"array","items":{"type":"string"}},"inferred_symptoms":{"type":"array","items":{"type":"string"}}},"required":["symptoms","negated_symptoms","medical_history","diagnosis","family_history","inferred_symptoms"],"additionalProperties":false}`), &s)
	return s
}
func matchSchema(candidates []CandidateTerm) map[string]any {
	ids := []any{}
	for _, c := range candidates {
		ids = append(ids, c.Term.ID)
	}
	return map[string]any{"type": "object", "properties": map[string]any{"terms": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{"hpo_id": map[string]any{"type": "string", "enum": ids}, "confidence": map[string]any{"type": "string", "enum": []any{"high", "medium", "low"}}, "original_symptom": map[string]any{"type": "string"}, "polarity": map[string]any{"type": "string", "enum": []any{"present", "absent", "uncertain", "family_history", "inferred"}}}, "required": []any{"hpo_id", "confidence", "original_symptom", "polarity"}, "additionalProperties": false}}}, "required": []any{"terms"}, "additionalProperties": false}
}
