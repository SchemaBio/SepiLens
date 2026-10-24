package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/pzweuj/sepilens/internal/hpo"
	"github.com/pzweuj/sepilens/internal/llm"
	"github.com/pzweuj/sepilens/internal/llm/responses"
)

func TestHPOV2AIExtractionBothProtocolsAndNoPersistence(t *testing.T) {
	for _, protocol := range []llm.Protocol{llm.ProtocolOpenAIChat, llm.ProtocolOpenAIResponses} {
		t.Run(string(protocol), func(t *testing.T) {
			s := newV2Server(t)
			s.hpoExtractor = hpo.NewExtractor(hpo.NewIndex(map[string]hpo.Term{
				"HP:0002321": {ID: "HP:0002321", Name: "Vertigo", NameCN: "眩晕"},
				"HP:0002315": {ID: "HP:0002315", Name: "Headache", NameCN: "头痛"},
			}), nil)
			var calls atomic.Int32
			model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				content := `{"symptoms":["眩晕"],"negated_symptoms":["头痛"],"medical_history":[],"diagnosis":[],"family_history":[],"inferred_symptoms":[]}`
				if n > 1 {
					content = `{"terms":[{"hpo_id":"HP:0002321","confidence":"high","original_symptom":"眩晕","polarity":"present"},{"hpo_id":"HP:0002315","confidence":"high","original_symptom":"头痛","polarity":"present"}]}`
				}
				if protocol == llm.ProtocolOpenAIChat {
					_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}, "finish_reason": "stop"}}})
				} else {
					_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output": []any{map[string]any{"content": []any{map[string]any{"type": "output_text", "text": content}}}}})
				}
			}))
			defer model.Close()
			if _, err := s.profiles.Save(context.Background(), "T", "hpo", responses.Config{BaseURL: model.URL, APIKey: "synthetic", Model: "synthetic", Protocol: protocol}); err != nil {
				t.Fatal(err)
			}
			req := HPOExtractRequest{Text: "患者眩晕，无头痛", Options: HPOExtractOptions{ProviderProfileID: "hpo"}}
			w := callV2(s, "POST", "/v2/hpo/extract", req, "key", "")
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			var result hpo.ExtractionResult
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.ExecutionMode != "model" || result.Degraded || calls.Load() != 2 || len(result.Terms) != 2 {
				t.Fatal(result, calls.Load())
			}
			for _, term := range result.Terms {
				if term.HPO == "HP:0002315" && term.Polarity != hpo.PolarityAbsent {
					t.Fatal("negation lost")
				}
			}
			var count int
			if err := s.db.QueryRow("SELECT COUNT(*) FROM jobs").Scan(&count); err != nil || count != 0 {
				t.Fatal("HPO original persisted in jobs", count, err)
			}
			if w := callV2(s, "POST", "/v2/hpo/extract", req, "other", ""); w.Code != 422 {
				t.Fatal("cross-tenant profile exposed", w.Code)
			}
			req.Options.AllowLocalFallback = true
			if w := callV2(s, "POST", "/v2/hpo/extract", req, "key", ""); w.Code != 422 {
				t.Fatal("local fallback allowed", w.Code)
			}
		})
	}
}

func TestHPOV2ProviderFailureNeverReturnsLocalExtraction(t *testing.T) {
	s := newV2Server(t)
	s.hpoExtractor = hpo.NewExtractor(hpo.NewIndex(map[string]hpo.Term{"HP:0002315": {ID: "HP:0002315", NameCN: "头痛"}}), nil)
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		fmt.Fprint(w, "sensitive provider failure")
	}))
	defer model.Close()
	_, err := s.profiles.Save(context.Background(), "T", "fail", responses.Config{BaseURL: model.URL, APIKey: "synthetic", Model: "synthetic", Protocol: llm.ProtocolOpenAIChat})
	if err != nil {
		t.Fatal(err)
	}
	w := callV2(s, "POST", "/v2/hpo/extract", HPOExtractRequest{Text: "头痛", Options: HPOExtractOptions{ProviderProfileID: "fail"}}, "key", "")
	if w.Code != 503 {
		t.Fatal(w.Code, w.Body.String())
	}
	var result hpo.ExtractionResult
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if len(result.Terms) > 0 {
		t.Fatal("local result returned after model failure")
	}
}
