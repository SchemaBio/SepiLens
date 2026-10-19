package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pzweuj/sepilens/internal/agent"
	"github.com/pzweuj/sepilens/internal/annotation"
	"github.com/pzweuj/sepilens/internal/config"
	"github.com/pzweuj/sepilens/internal/engine"
	"github.com/pzweuj/sepilens/internal/jobs"
	"github.com/pzweuj/sepilens/internal/llm"
	"github.com/pzweuj/sepilens/internal/llm/responses"
	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
	"github.com/pzweuj/sepilens/internal/storage/sqlite"
)

func TestAIJobPreservesBaselineSuccessAndFailure(t *testing.T) {
	for _, protocol := range []llm.Protocol{llm.ProtocolOpenAIResponses, llm.ProtocolOpenAIChat} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprint(protocol, "/failure=", fail), func(t *testing.T) {
				calls := 0
				model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					var body map[string]any
					_ = json.NewDecoder(r.Body).Decode(&body)
					if fail {
						w.WriteHeader(500)
						fmt.Fprint(w, "secret patient provider response")
						return
					}
					if calls == 1 {
						if protocol == llm.ProtocolOpenAIResponses {
							fmt.Fprint(w, `{"status":"completed","output":[{"type":"function_call","call_id":"fixed-call","name":"get_annotations","arguments":"{}"}],"usage":{"input_tokens":3,"output_tokens":1}}`)
						} else {
							fmt.Fprint(w, `{"choices":[{"message":{"tool_calls":[{"id":"fixed-call","type":"function","function":{"name":"get_annotations","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":1}}`)
						}
						return
					}
					content := `{"criteria":[],"classification":"Pathogenic"}`
					if protocol == llm.ProtocolOpenAIResponses {
						_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output": []any{map[string]any{"content": []any{map[string]any{"type": "output_text", "text": content}}}}})
					} else {
						_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}, "finish_reason": "stop"}}})
					}
				}))
				defer model.Close()
				db, e := sqlite.Open(filepath.Join(t.TempDir(), "ai.db"))
				if e != nil {
					t.Fatal(e)
				}
				defer db.Close()
				cfg := config.Config{MasterKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", AIMode: "enabled", AuthMode: "required", AllowPrivateEndpoints: true, RetentionDays: 90, Principals: map[string]config.Principal{"token": {Tenant: "T", Actor: "admin", Role: "admin"}}}
				s := NewServer(cfg, jobs.NewSQLiteStore(db), db, slog.New(slog.NewTextHandler(io.Discard, nil)))
				s.SetACMGEvaluator(acmg2015.NewEvaluator(nil))
				s.approvals = map[engine.EngineID]engine.Approval{engine.EngineACMG2015: {ContentHash: "synthetic"}}
				if e = s.Start(context.Background()); e != nil {
					t.Fatal(e)
				}
				defer s.Close()
				if _, e = s.profiles.Save(context.Background(), "T", "mock", responses.Config{BaseURL: model.URL, APIKey: "synthetic", Model: "synthetic", Protocol: protocol}); e != nil {
					t.Fatal(e)
				}
				body := `{"engine_id":"acmg_amp_2015","mode":"ai_assisted","variant":{"assembly":"GRCh38","chrom":"1","pos":1,"ref":"A","alt":"G","gene":"TEST"},"clinical_context":{"disease_id":"OMIM:1","inheritance":"AD","variant_origin":"germline_confirmed","sample_type":"blood"},"annotations":{"source":"sepilens_snapshot","alphamissense":{"version":"synthetic","score":0.99,"class":"likely_pathogenic"}},"options":{"provider_profile_id":"mock","retrieval_mode":"off"}}`
				r := httptest.NewRequest("POST", "/v1/variants/snv/evaluations", strings.NewReader(body))
				r.Header.Set("X-API-Key", "token")
				w := httptest.NewRecorder()
				s.Router().ServeHTTP(w, r)
				if w.Code != 202 {
					t.Fatal(w.Code, w.Body.String())
				}
				var accepted struct {
					ID string `json:"job_id"`
				}
				_ = json.Unmarshal(w.Body.Bytes(), &accepted)
				deadline := time.Now().Add(8 * time.Second)
				var j jobs.Job
				for {
					j, e = s.store.GetJob(context.Background(), "T", accepted.ID)
					if e != nil {
						t.Fatal(e)
					}
					if j.Status == jobs.StatusSucceeded || j.Status == jobs.StatusFailed {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("AI worker timeout")
					}
					time.Sleep(10 * time.Millisecond)
				}
				var result EvaluationResultPayload
				if e = json.Unmarshal([]byte(j.ResultJSON), &result); e != nil {
					t.Fatal("partial result not saved", j, e)
				}
				if result.LocalBaseline == nil || result.SettlementEligible || result.PromptVersion != agent.PromptVersion {
					t.Fatal(result)
				}
				if fail {
					if j.Status != jobs.StatusFailed || result.DeliveryStatus != "failed" || result.FailureCode != "AI_PROVIDER_UNAVAILABLE" {
						t.Fatal(j, result)
					}
				} else if j.Status != jobs.StatusSucceeded || result.RuleEngineResult.(map[string]any)["classification"] == "Pathogenic" {
					t.Fatal("model classification trusted", result)
				}
				snap, e := s.evidenceStore.GetSnapshot(context.Background(), "T", accepted.ID)
				if e != nil || snap.HashVersion != "v2" || len(snap.Agent) == 0 {
					t.Fatal(snap, e)
				}
				var a annotation.Set
				if json.Unmarshal(snap.Annotations, &a) != nil || a.Source != annotation.SourceCaller {
					t.Fatal("caller source forgery trusted")
				}
				r = httptest.NewRequest("GET", "/v1/jobs/"+accepted.ID+"/result", nil)
				r.Header.Set("X-API-Key", "token")
				w = httptest.NewRecorder()
				s.Router().ServeHTTP(w, r)
				if w.Code != 200 || !strings.Contains(w.Body.String(), "local_baseline") || strings.Contains(w.Body.String(), "secret patient") {
					t.Fatal(w.Code, w.Body.String())
				}
			})
		}
	}
}
