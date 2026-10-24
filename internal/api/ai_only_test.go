package api

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/pzweuj/sepilens/internal/engine"
	"github.com/pzweuj/sepilens/internal/jobs"
	"github.com/pzweuj/sepilens/internal/llm"
	"github.com/pzweuj/sepilens/internal/llm/responses"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestAIOnlyOptionalApprovalKeepsFormalRulesDisabled(t *testing.T) {
	for _, id := range []string{"acmg_amp_2015", "cnv_gain_2020", "cnv_loss_2020"} {
		t.Run(id, func(t *testing.T) {
			s := newV2Server(t)
			s.approvals = map[engine.EngineID]engine.Approval{}
			s.cfg.AIOnly = true
			req := v2Fixture(id)
			if w := callV2(s, "POST", "/v2/evaluations", req, "key", ""); w.Code != 403 {
				t.Fatal("local mode exposed", w.Code)
			}
			req.Mode = "ai_assisted"
			req.Options.ProviderProfileID = "mock"
			if w := callV2(s, "POST", "/v2/evaluations", req, "key", ""); w.Code != 409 {
				t.Fatal("default approval gate changed", w.Code)
			}
			s.cfg.AllowUnapprovedAI = true
			var calls atomic.Int32
			model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				content := fmt.Sprintf("%s", `{"criteria":[],"classification":"Pathogenic"}`)
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}, "finish_reason": "stop"}}})
			}))
			defer model.Close()
			if _, err := s.profiles.Save(context.Background(), "T", "mock", responses.Config{BaseURL: model.URL, APIKey: "synthetic", Model: "synthetic", Protocol: llm.ProtocolOpenAIChat}); err != nil {
				t.Fatal(err)
			}
			w := callV2(s, "POST", "/v2/evaluations", req, "key", "")
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
			if j.Status != jobs.StatusSucceeded || calls.Load() == 0 || result.AgentProposal == nil || result.ResultKind != "evidence_only" || result.ClinicalUse != "forbidden" || result.SettlementEligible {
				t.Fatal(j.Status, result)
			}
			for _, cap := range s.descriptors() {
				if string(cap.ID) == id && (cap.Enabled || !cap.AIAvailable) {
					t.Fatal("formal rule approval confused with AI availability", cap)
				}
			}
		})
	}
}
