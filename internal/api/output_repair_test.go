package api

import (
	"context"
	"encoding/json"
	"github.com/pzweuj/sepilens/internal/assessment"
	"github.com/pzweuj/sepilens/internal/engine"
	"github.com/pzweuj/sepilens/internal/jobs"
	"github.com/pzweuj/sepilens/internal/llm"
	"github.com/pzweuj/sepilens/internal/llm/responses"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSVCOutputFormatCorrectionPreservesDeliveryAndFailureGates(t *testing.T) {
	for _, badTwice := range []bool{false, true} {
		t.Run(map[bool]string{false: "corrected", true: "invalid_twice"}[badTwice], func(t *testing.T) {
			s := newV2Server(t)
			mockSVCSearch(s)
			s.approvals = map[engine.EngineID]engine.Approval{}
			s.cfg.BillSVCAI = true
			var calls atomic.Int32
			valid := svcTestContext(`{"clinical_summary":"Synthetic SVC summary","criteria":[{"code":"PFD","status":"insufficient_data","strength":"","score_hundredths":null,"points":null,"source_ids":[],"facts_confirmed":[],"change":"confirmed","override_reason":"Method inputs unavailable"}],"classification":"VUS"}`, true)
			model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count := calls.Add(1)
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				if count == 2 && body["tools"] != nil {
					t.Error("tools enabled during correction")
				}
				content := valid
				if count == 1 || badTwice {
					content = strings.Replace(valid, `"points":null`, `"points":12`, 1)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}, "finish_reason": "stop"}}})
			}))
			defer model.Close()
			if _, err := s.profiles.Save(context.Background(), "T", "format-test", responses.Config{BaseURL: model.URL, APIKey: "synthetic", Model: "synthetic", Protocol: llm.ProtocolOpenAIChat}); err != nil {
				t.Fatal(err)
			}
			r := v2Fixture("svc_v4")
			r.Mode = "ai_assisted"
			r.Options.AllowExperimental = true
			r.Options.ProviderProfileID = "format-test"
			r.Package.Sources = nil
			r.Package.Draft = nil
			r.Package.DraftSourceIDs = nil
			r.Package.AIContext = &assessment.AIContext{Sample: &assessment.AISampleContext{AnalysisMode: "trio"}}
			r.Context.DiseaseID = ""
			r.Context.Inheritance = "unknown"
			r.Context.VariantOrigin = "unknown"
			w := callV2(s, "POST", "/v2/evaluations", r, "key", "format")
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
			if calls.Load() != 2 || result.RuleEngineResult != nil || result.ClinicalUse != "forbidden" {
				t.Fatal("correction changed execution boundaries")
			}
			if badTwice {
				if result.FailureCode != "AI_OUTPUT_INVALID" || result.SettlementEligible || result.DeliveryStatus != "failed" {
					t.Fatal("invalid retry adopted")
				}
			} else if j.Status != jobs.StatusSucceeded || result.DeliveryStatus != "delivered" || !result.SettlementEligible {
				t.Fatal("valid correction not delivered", result.FailureCode)
			}
		})
	}
}
