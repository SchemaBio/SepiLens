package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/pzweuj/sepilens/internal/assessment"
	"github.com/pzweuj/sepilens/internal/engine"
	"github.com/pzweuj/sepilens/internal/jobs"
	"github.com/pzweuj/sepilens/internal/llm"
	"github.com/pzweuj/sepilens/internal/llm/responses"
	"github.com/pzweuj/sepilens/internal/retrieval"
)

func TestClaimBindingCorrectionAndSelectiveProjectionAcrossEngines(t *testing.T) {
	for _, id := range []string{"acmg_amp_2015", "svc_v4", "cnv_loss_2020", "cnv_gain_2020"} {
		for _, invalidTwice := range []bool{false, true} {
			t.Run(id+map[bool]string{true: "_projected", false: "_corrected"}[invalidTwice], func(t *testing.T) {
				s := newV2Server(t)
				s.approvals = map[engine.EngineID]engine.Approval{}
				s.cfg.AIEvidenceChain = true
				s.cfg.AICandidateEngines = map[string]bool{id: true}
				s.cfg.BillACMGAI = true
				s.cfg.BillSVCAI = true
				s.cfg.BillCNVAI = true
				s.coordinator = retrieval.NewSearchCoordinator(retrieval.CoordinatorConfig{}, []retrieval.Retriever{retrieval.NewPubMedRetriever(true, nil)})
				var calls atomic.Int32
				model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					n := calls.Add(1)
					if n > 2 {
						t.Error("second correction attempted")
					}
					var request map[string]any
					_ = json.NewDecoder(r.Body).Decode(&request)
					encoded, _ := json.Marshal(request)
					if n == 2 && (request["tools"] != nil || !strings.Contains(string(encoded), "criteria[0].evidence_bindings")) {
						t.Error("correction lost fixed diagnostics or enabled tools")
					}
					code, strength := "PM2", "Moderate"
					var score any
					facts := []string{"synthetic_fact"}
					fact := "synthetic_fact"
					if id == "svc_v4" {
						code = "PFD"
						strength = ""
						facts = []string{}
						fact = ""
					}
					if strings.HasPrefix(id, "cnv_") {
						code = "2A"
						strength = ""
						score = int64(100)
					}
					clause := map[string]any{"code": code, "status": "met", "strength": strength, "score_hundredths": score, "points": nil, "source_ids": []string{"src_999"}, "facts_confirmed": facts, "change": "added", "override_reason": "Synthetic unsupported claim", "evidence_bindings": []any{map[string]any{"fact": fact, "fact_id": "never-supplied", "source_id": "src_999", "excerpt": ""}}}
					if n == 2 && !invalidTwice {
						clause["status"] = "insufficient_data"
						clause["score_hundredths"] = nil
						clause["source_ids"] = []string{}
						clause["facts_confirmed"] = []string{}
						clause["evidence_bindings"] = []any{}
					}
					content := map[string]any{"clinical_summary": "Synthetic organized context; independent support unavailable.", "classification": "VUS", "criteria": []any{clause}}
					if strings.HasPrefix(id, "cnv_") {
						content["cnv_context"] = map[string]any{"disease_id": "", "inheritance": "unknown", "rationale": "Synthetic missing evidence", "source_ids": []string{}}
					}
					raw, _ := json.Marshal(content)
					text := svcTestContext(string(raw), id == "svc_v4")
					_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": text}, "finish_reason": "stop"}}, "usage": map[string]any{"prompt_tokens": 2, "completion_tokens": 3}})
				}))
				defer model.Close()
				if _, err := s.profiles.Save(context.Background(), "T", "mock", responses.Config{BaseURL: model.URL, APIKey: "synthetic", Model: "synthetic", Protocol: llm.ProtocolOpenAIChat}); err != nil {
					t.Fatal(err)
				}
				request := v2Fixture(id)
				request.Mode = "ai_assisted"
				request.Package.Sources = nil
				request.Package.Criteria = nil
				request.Package.Draft = nil
				request.Package.DraftSourceIDs = nil
				request.Package.AIContext = &assessment.AIContext{}
				request.Options.ProviderProfileID = "mock"
				request.Options.AllowExperimental = id == "svc_v4"
				response := callV2(s, "POST", "/v2/evaluations", request, "key", "")
				if response.Code != 202 {
					t.Fatal(response.Code, response.Body.String())
				}
				var accepted struct {
					ID string `json:"job_id"`
				}
				_ = json.Unmarshal(response.Body.Bytes(), &accepted)
				job := waitV2(t, s, accepted.ID)
				var result EvaluationResultPayload
				_ = json.Unmarshal([]byte(job.ResultJSON), &result)
				if job.Status != jobs.StatusSucceeded || calls.Load() != 2 || result.Usage.InputTokens != 4 || result.Usage.OutputTokens != 6 || result.ClinicalUse != "forbidden" {
					t.Fatal("shared correction/delivery contract failed", job.Status, calls.Load(), result.FailureCode)
				}
				proposal := result.AgentProposal.(map[string]any)
				ai := proposal["ai_assessment"].(map[string]any)
				clauses := ai["criteria"].([]any)
				if clauses[0].(map[string]any)["status"] != "insufficient_data" {
					t.Fatal("unsupported medical claim delivered")
				}
				if invalidTwice && ai["classification"] != "insufficient_data" {
					t.Fatal("projected result retained misleading classification")
				}
				if proposal["evidence_quality"].(map[string]any)["evidence_bindings"] == nil {
					t.Fatal("binding audit missing")
				}
				if id == "svc_v4" && result.RuleEngineResult != nil {
					t.Fatal("experimental SVC became formal result")
				}
			})
		}
	}
}
