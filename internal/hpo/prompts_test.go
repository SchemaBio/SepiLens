package hpo_test

import (
	"path/filepath"
	"testing"

	"github.com/pzweuj/sepilens/internal/hpo"
)

func TestPrompts_PreprocessParsing(t *testing.T) {
	rawMarkdown := "```json\n" + `{
  "symptoms": ["眩晕", "头痛"],
  "medicalHistory": ["高血压"],
  "diagnosis": ["急性脑梗死"],
  "negatedSymptoms": ["发热"],
  "familyHistory": ["父亲冠心病"]
}` + "\n```"

	res, err := hpo.ParsePreprocessJSON(rawMarkdown)
	if err != nil {
		t.Fatalf("failed to parse preprocess markdown: %v", err)
	}

	if len(res.Symptoms) != 2 || res.Symptoms[0] != "眩晕" {
		t.Fatalf("unexpected symptoms: %+v", res.Symptoms)
	}
	if len(res.MedicalHistory) != 1 || res.MedicalHistory[0] != "高血压" {
		t.Fatalf("unexpected medical history: %+v", res.MedicalHistory)
	}
	if len(res.NegatedSymptoms) != 1 || res.NegatedSymptoms[0] != "发热" {
		t.Fatalf("unexpected negated symptoms: %+v", res.NegatedSymptoms)
	}
	if len(res.FamilyHistory) != 1 || res.FamilyHistory[0] != "父亲冠心病" {
		t.Fatalf("unexpected family history: %+v", res.FamilyHistory)
	}

	keywords := hpo.ExtractQueryKeywords(res)
	if len(keywords) < 4 {
		t.Fatalf("expected at least 4 keywords, got %d: %+v", len(keywords), keywords)
	}
}

func TestPrompts_MatchResponseParsingAndSecurityFilter(t *testing.T) {
	kDir := filepath.Join("..", "..", "knowledge", "hpo")
	idx, err := hpo.LoadDefaultIndex(kDir)
	if err != nil {
		t.Fatalf("failed to load index: %v", err)
	}

	rawMatch := `[
		{
			"hpo_id": "HP:0002321",
			"confidence": "高",
			"remark": "【眩晕】→ 精确匹配"
		},
		{
			"hpo_id": "HP:9999999",
			"confidence": "高",
			"remark": "【虚构症状】→ 模型编造的ID"
		}
	]`

	items, err := hpo.ParseMatchResponse(rawMatch, idx)
	if err != nil {
		t.Fatalf("failed to parse match response: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("expected hallucinated HP:9999999 to be dropped, got %d items", len(items))
	}

	if items[0].HPO != "HP:0002321" || items[0].Confidence != hpo.ConfidenceHigh {
		t.Fatalf("unexpected matched item: %+v", items[0])
	}
	if items[0].OriginalSymptom != "眩晕" {
		t.Fatalf("expected original symptom 眩晕, got %q", items[0].OriginalSymptom)
	}
}
