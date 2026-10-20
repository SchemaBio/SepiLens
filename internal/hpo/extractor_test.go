package hpo_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pzweuj/sepilens/internal/hpo"
	"github.com/pzweuj/sepilens/internal/llm"
)

type mockLLM struct {
	calls []string
}

func (m *mockLLM) Capabilities(ctx context.Context) (llm.ModelCapabilities, error) {
	return llm.ModelCapabilities{StructuredOutput: true}, nil
}

func (m *mockLLM) Generate(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResult, error) {
	m.calls = append(m.calls, req.Prompt)
	if len(m.calls) == 1 {
		return llm.GenerateResult{Content: `{"symptoms":["眩晕"],"negated_symptoms":["头痛"],"medical_history":["高血压"],"diagnosis":[],"family_history":[],"inferred_symptoms":[]}`}, nil
	}
	return llm.GenerateResult{Content: `{"terms":[{"hpo_id":"HP:0002321","confidence":"high","original_symptom":"眩晕","polarity":"present"},{"hpo_id":"HP:0002315","confidence":"high","original_symptom":"头痛","polarity":"present"}]}`}, nil
}

func (m *mockLLM) Stream(ctx context.Context, req llm.GenerateRequest) (<-chan llm.ModelEvent, error) {
	ch := make(chan llm.ModelEvent)
	close(ch)
	return ch, nil
}

func TestExtractor_FullPipelineAndNegationFilter(t *testing.T) {
	kDir := filepath.Join("..", "..", "tests", "fixtures", "hpo", "reference")
	idx, err := hpo.LoadDefaultIndex(kDir)
	if err != nil {
		t.Fatalf("failed to load default index: %v", err)
	}

	mock := &mockLLM{}
	extractor := hpo.NewExtractor(idx, mock)

	var stages []string
	res, err := extractor.Extract(context.Background(), "患者眩晕，无头痛，既往高血压", hpo.ExtractOptions{
		OnStage: func(st string, payload any) {
			stages = append(stages, st)
		},
	})
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	// Verify stage progression
	if len(stages) < 3 {
		t.Fatalf("expected multiple stage events, got %+v", stages)
	}

	// Verify matched terms: HP:0002321 Vertigo must be present
	foundVertigo := false
	for _, term := range res.Terms {
		if term.HPO == "HP:0002321" {
			foundVertigo = true
		}
		// Negation filter: Headache HP:0002315 must have been filtered out!
		if (term.HPO == "HP:0002315" || term.NameCN == "头痛") && term.Polarity != hpo.PolarityAbsent {
			t.Fatalf("CRITICAL: negated symptom Headache (HP:0002315) was not filtered out!")
		}
	}

	if !foundVertigo {
		t.Fatalf("expected HP:0002321 Vertigo in extraction results, got: %+v", res.Terms)
	}
}

func TestExtractor_RuleBasedFallback(t *testing.T) {
	kDir := filepath.Join("..", "..", "tests", "fixtures", "hpo", "reference")
	idx, err := hpo.LoadDefaultIndex(kDir)
	if err != nil {
		t.Fatalf("failed to load default index: %v", err)
	}

	extractor := hpo.NewExtractor(idx, nil) // no LLM provider

	res, err := extractor.Extract(context.Background(), "多囊性肾发育不良", hpo.ExtractOptions{AllowLocalFallback: true})
	if err != nil {
		t.Fatalf("fallback extraction failed: %v", err)
	}

	if len(res.Terms) == 0 {
		t.Fatalf("expected fallback extraction to find terms, got 0")
	}
	if res.Terms[0].HPO != "HP:0000003" {
		t.Fatalf("expected HP:0000003 for 多囊性肾发育不良, got %s", res.Terms[0].HPO)
	}
}
