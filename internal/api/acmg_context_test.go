package api

import (
	"context"
	"encoding/json"
	"github.com/pzweuj/sepilens/internal/annotation"
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

func TestACMGAIReportsUnavailableLiteratureWithoutLocalFallback(t *testing.T) {
	s := newV2Server(t)
	s.approvals = map[engine.EngineID]engine.Approval{}
	s.cfg.AllowUnapprovedAI = true
	var calls atomic.Int32
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		encoded, _ := json.Marshal(body["messages"])
		if !strings.Contains(string(encoded), "synthetic clinical background") || !strings.Contains(string(encoded), "SIFT_pred") {
			t.Error("imported clinical context missing from model")
		}
		if calls.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"id":"search","type":"function","function":{"name":"search_literature","arguments":"{}"}},{"id":"phenotype","type":"function","function":{"name":"get_phenotype","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`))
			return
		}
		if !strings.Contains(string(encoded), `unavailable`) || !strings.Contains(string(encoded), `unscored_context`) || !strings.Contains(string(encoded), `not_run`) {
			t.Error("retrieval failure hidden from model")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": `{"clinical_summary":"Synthetic clinical context organized; inheritance unknown.","criteria":[{"code":"PM2","status":"insufficient_data","strength":"Moderate","score_hundredths":null,"points":null,"source_ids":[],"facts_confirmed":[],"change":"confirmed","override_reason":"coverage unavailable"}],"classification":"VUS"}`}, "finish_reason": "stop"}}})
	}))
	defer model.Close()
	_, err := s.profiles.Save(context.Background(), "T", "mock", responses.Config{BaseURL: model.URL, APIKey: "synthetic", Model: "synthetic", Protocol: llm.ProtocolOpenAIChat})
	if err != nil {
		t.Fatal(err)
	}
	r := v2Fixture("acmg_amp_2015")
	r.Mode = "ai_assisted"
	r.Context.DiseaseID = ""
	r.Context.Inheritance = ""
	r.Context.SampleType = ""
	r.Context.VariantOrigin = ""
	r.Package.AIContext = &assessment.AIContext{VariantAnnotations: map[string]any{"SIFT_pred": "deleterious"}, Sample: &assessment.AISampleContext{ClinicalDiagnosis: assessment.AIClinicalDiagnosis{MainDiagnosis: "synthetic clinical background"}}}
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
	proposal, _ := result.AgentProposal.(map[string]any)
	if j.Status != jobs.StatusSucceeded || calls.Load() != 2 || proposal["ai_assessment"] == nil || proposal["retrieval_status"] != "unavailable" || result.SettlementEligible {
		t.Fatal(j.Status, result)
	}
}

func TestACMGModelInputKeepsSampleHPOUnverified(t *testing.T) {
	r := v2Fixture("acmg_amp_2015")
	r.Context.HPOTerms = []string{"HP:0001250"}
	r.Package.Annotations = &annotation.Set{Source: annotation.SourceCaller, ClinVar: &annotation.ClinVar{Record: annotation.Record{Source: annotation.SourceCaller, Status: annotation.StatusCallerInput, Version: "unverified"}, Classification: "Uncertain significance", ReviewStatus: "criteria provided"}}
	input := modelAssessmentInput(r, assessment.Result{}, nil)
	if input["unverified_hpo_terms"].([]string)[0] != "HP:0001250" || input["phenotype"] != nil {
		t.Fatal("HPO context became verified phenotype")
	}
	cv := input["annotations"].(map[string]any)["clinvar"].(map[string]any)
	if cv["classification"] != "Uncertain significance" || cv["status"] != annotation.StatusCallerInput {
		t.Fatal("ClinVar context lost provenance")
	}
	if len(input["local_criteria"].([]map[string]any)) != 0 {
		t.Fatal("context invented evidence")
	}
}
