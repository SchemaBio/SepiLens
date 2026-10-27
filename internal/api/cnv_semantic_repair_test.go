package api

import (
	"context"
	"encoding/json"
	"github.com/pzweuj/sepilens/internal/assessment"
	"github.com/pzweuj/sepilens/internal/engine"
	"github.com/pzweuj/sepilens/internal/jobs"
	"github.com/pzweuj/sepilens/internal/llm"
	"github.com/pzweuj/sepilens/internal/llm/responses"
	"github.com/pzweuj/sepilens/internal/retrieval"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCNVSemanticRepairMissingEvidenceAndUnmetScores(t *testing.T) {
	for _, id := range []string{"cnv_loss_2020", "cnv_gain_2020"} {
		for _, invalidTwice := range []bool{false, true} {
			t.Run(id+map[bool]string{false: "_corrected", true: "_invalid_twice"}[invalidTwice], func(t *testing.T) {
				s := newV2Server(t)
				s.approvals = map[engine.EngineID]engine.Approval{}
				s.cfg.AICandidateEngines = map[string]bool{id: true}
				s.cfg.BillCNVAI = true
				var searches, calls atomic.Int32
				s.coordinator = retrieval.NewSearchCoordinator(retrieval.CoordinatorConfig{}, []retrieval.Retriever{cnvSyntheticRetriever{calls: &searches}})
				model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					n := calls.Add(1)
					var body map[string]any
					_ = json.NewDecoder(r.Body).Decode(&body)
					if n == 2 {
						b, _ := json.Marshal(body)
						if body["tools"] != nil || !strings.Contains(string(b), "criteria[0]") || !strings.Contains(string(b), "criteria[1]") {
							t.Error("semantic correction lost diagnostics or enabled tools")
						}
					}
					content := `{"clinical_summary":"Synthetic insufficient evidence.","cnv_context":{"disease_id":"","inheritance":"unknown","rationale":"unknown","source_ids":[]},"criteria":[{"code":"1A","status":"met","strength":"","score_hundredths":0,"points":null,"source_ids":[],"facts_confirmed":[],"change":"confirmed","override_reason":"Synthetic"},{"code":"1B","status":"not_met","strength":"","score_hundredths":0,"points":null,"source_ids":[],"facts_confirmed":[],"change":"confirmed","override_reason":"Synthetic"}],"classification":"insufficient_data"}`
					if n == 2 && !invalidTwice {
						content = `{"clinical_summary":"Synthetic insufficient evidence.","cnv_context":{"disease_id":"","inheritance":"unknown","rationale":"unknown","source_ids":[]},"criteria":[{"code":"1A","status":"insufficient_data","strength":"","score_hundredths":null,"points":null,"source_ids":[],"facts_confirmed":[],"change":"confirmed","override_reason":"No traceable evidence"}],"classification":"insufficient_data"}`
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}, "finish_reason": "stop"}}, "usage": map[string]any{"prompt_tokens": 2, "completion_tokens": 3}})
				}))
				defer model.Close()
				if _, err := s.profiles.Save(context.Background(), "T", "mock", responses.Config{BaseURL: model.URL, APIKey: "synthetic", Model: "synthetic", Protocol: llm.ProtocolOpenAIChat}); err != nil {
					t.Fatal(err)
				}
				r := v2Fixture(id)
				r.Mode = "ai_assisted"
				r.Package.Sources = nil
				r.Package.Criteria = nil
				r.Package.AIContext = &assessment.AIContext{}
				r.Options.ProviderProfileID = "mock"
				w := callV2(s, "POST", "/v2/evaluations", r, "key", "")
				if w.Code != 202 {
					t.Fatal(w.Code, w.Body.String())
				}
				var accepted struct {
					ID string `json:"job_id"`
				}
				_ = json.Unmarshal(w.Body.Bytes(), &accepted)
				j := waitV2(t, s, accepted.ID)
				var result EvaluationResultPayload
				_ = json.Unmarshal([]byte(j.ResultJSON), &result)

				if calls.Load() != 1 {
					t.Fatal("defaulting must not rerun a paid model", calls.Load())
				}
				if j.Status != jobs.StatusSucceeded || result.ClinicalUse != "forbidden" || result.Usage.InputTokens != 2 || result.Usage.OutputTokens != 3 {
					t.Fatal("default draft failed", j.Status, result.FailureCode)
				}
				p := result.AgentProposal.(map[string]any)
				if p["calculator_draft"] != true || len(p["calculator_defaults"].([]any)) == 0 {
					t.Fatal("default audit missing")
				}
				for _, item := range p["ai_assessment"].(map[string]any)["criteria"].([]any) {
					c := item.(map[string]any)
					if c["status"] == "met" || c["score_hundredths"] != nil {
						t.Fatal("unsupported model assertion adopted")
					}
				}

			})
		}
	}
}
