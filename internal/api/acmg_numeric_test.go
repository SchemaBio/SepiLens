package api

import (
	"context"
	"encoding/json"
	"github.com/pzweuj/sepilens/internal/jobs"
	"github.com/pzweuj/sepilens/internal/llm"
	"github.com/pzweuj/sepilens/internal/llm/responses"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestACMGRedundantNumbersNeverControlEvidenceScores(t *testing.T) {
	s := newV2Server(t)
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := `{"criteria":[{"code":"PM2","status":"met","strength":"Moderate","score_hundredths":999900,"points":"9999","source_ids":[],"facts_confirmed":[],"change":"upgraded_from_insufficient_data","override_reason":"synthetic model candidate"},{"code":"BP1","status":"not_applicable","strength":"Supporting","score_hundredths":0,"points":"0","source_ids":[],"facts_confirmed":[],"change":"confirmed","override_reason":"synthetic non-met candidate"}],"classification":"VUS"}`
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}, "finish_reason": "stop"}}})
	}))
	defer model.Close()
	if _, err := s.profiles.Save(context.Background(), "T", "mock", responses.Config{BaseURL: model.URL, APIKey: "synthetic", Model: "synthetic", Protocol: llm.ProtocolOpenAIChat}); err != nil {
		t.Fatal(err)
	}
	r := v2Fixture("acmg_amp_2015")
	r.Mode = "ai_assisted"
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
	if j.Status != jobs.StatusSucceeded || result.DeliveryStatus != "delivered" {
		t.Fatal(j.Status, result.FailureCode)
	}
	proposal := result.AgentProposal.(map[string]any)
	criteria := proposal["ai_assessment"].(map[string]any)["criteria"].([]any)
	for _, raw := range criteria {
		criterion := raw.(map[string]any)
		if criterion["score_hundredths"] != nil || criterion["points"] != nil {
			t.Fatal("model numbers leaked into accepted proposal", criterion)
		}
	}
	if len(proposal["normalization_warnings"].([]any)) != 1 {
		t.Fatal("ignored arithmetic not disclosed")
	}
	for _, item := range proposal["reconciliation"].([]any) {
		entry := item.(map[string]any)
		if entry["code"] == "PM2" && entry["accepted"] == true {
			t.Fatal("missing provenance bypassed")
		}
	}
}
