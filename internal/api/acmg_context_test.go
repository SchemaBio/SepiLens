package api

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestSequenceAIReportsUnavailableLiteratureWithoutLocalFallback(t *testing.T) {
	for _, engineID := range []string{"acmg_amp_2015", "svc_v4"} {
		t.Run(engineID, func(t *testing.T) {
			s := newV2Server(t)
			s.approvals = map[engine.EngineID]engine.Approval{}
			s.cfg.BillACMGAI = true
			s.cfg.BillSVCAI = true
			s.cfg.AllowUnapprovedAI = false
			s.cfg.AICandidateEngines = map[string]bool{"acmg_amp_2015": true}
			var calls atomic.Int32
			model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				encoded, _ := json.Marshal(body["messages"])
				if !strings.Contains(string(encoded), "synthetic clinical background") || !strings.Contains(string(encoded), "SIFT_pred") || !strings.Contains(string(encoded), "analysis_mode") || !strings.Contains(string(encoded), "trio") || !strings.Contains(string(encoded), "sampleGenotypes") || !strings.Contains(string(encoded), "father") {
					t.Error("imported clinical context missing from model")
				}
				if calls.Add(1) == 1 {
					_, _ = w.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"id":"search","type":"function","function":{"name":"search_literature","arguments":"{}"}},{"id":"phenotype","type":"function","function":{"name":"get_phenotype","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`))
					return
				}
				if !strings.Contains(string(encoded), `unavailable`) || !strings.Contains(string(encoded), `unscored_context`) || !strings.Contains(string(encoded), `not_run`) {
					t.Error("retrieval failure hidden from model")
				}
				code, strength := "PM2", "Moderate"
				if engineID == "svc_v4" {
					code, strength = "PFD", ""
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": strings.ReplaceAll(strings.ReplaceAll(`{"clinical_summary":"Synthetic clinical context organized; inheritance unknown.","criteria":[{"code":"ENGINE_CODE","status":"insufficient_data","strength":"ENGINE_STRENGTH","score_hundredths":null,"points":null,"source_ids":[],"facts_confirmed":[],"change":"confirmed","override_reason":"coverage unavailable"}],"classification":"VUS"}`, "ENGINE_CODE", code), "ENGINE_STRENGTH", strength)}, "finish_reason": "stop"}}})
			}))
			defer model.Close()
			_, err := s.profiles.Save(context.Background(), "T", "mock", responses.Config{BaseURL: model.URL, APIKey: "synthetic", Model: "synthetic", Protocol: llm.ProtocolOpenAIChat})
			if err != nil {
				t.Fatal(err)
			}
			r := v2Fixture(engineID)
			r.Package.Sources = nil
			r.Package.Draft = nil
			r.Package.DraftSourceIDs = nil
			r.Options.AllowExperimental = engineID == "svc_v4"
			r.Mode = "ai_assisted"
			r.Context.DiseaseID = ""
			r.Context.Inheritance = ""
			r.Context.SampleType = ""
			r.Context.VariantOrigin = ""
			r.Package.AIContext = &assessment.AIContext{VariantAnnotations: map[string]any{"SIFT_pred": "deleterious", "sampleGenotypes": map[string]any{"proband": map[string]any{"genotype": "1|0"}, "father": map[string]any{"genotype": "0/0", "alleleFrequency": 0}, "mother": map[string]any{"genotype": "0/1"}}}, Sample: &assessment.AISampleContext{AnalysisMode: "trio", ClinicalDiagnosis: assessment.AIClinicalDiagnosis{MainDiagnosis: "synthetic clinical background"}}}
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
			if result.ClinicalUse != "forbidden" {
				t.Fatal("inferred context became clinical output")
			}
			if engineID == "svc_v4" && (result.ResultKind != "experimental" || result.RuleEngineResult != nil) {
				t.Fatal("SVC lost experimental boundary")
			}
		})
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

func TestSVCCommercialAIKeepsDraftIdentity(t *testing.T) {
	for _, paid := range []bool{false, true} {
		t.Run(fmt.Sprint(paid), func(t *testing.T) {
			s := newV2Server(t)
			s.approvals = map[engine.EngineID]engine.Approval{}
			s.cfg.BillSVCAI = paid
			model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": `{"clinical_summary":"Synthetic SVC analysis","criteria":[{"code":"PFD","status":"insufficient_data","strength":"","score_hundredths":null,"points":null,"source_ids":[],"facts_confirmed":[],"change":"confirmed","override_reason":"Method material unavailable"}],"classification":"VUS"}`}, "finish_reason": "stop"}}})
			}))
			defer model.Close()
			if _, e := s.profiles.Save(context.Background(), "T", "paid-svc", responses.Config{BaseURL: model.URL, APIKey: "synthetic", Model: "synthetic", Protocol: llm.ProtocolOpenAIChat}); e != nil {
				t.Fatal(e)
			}
			r := v2Fixture("svc_v4")
			r.Mode = "ai_assisted"
			r.Options.AllowExperimental = true
			r.Options.ProviderProfileID = "paid-svc"
			r.Package.Sources = nil
			r.Package.Draft = nil
			r.Package.DraftSourceIDs = nil
			r.Package.AIContext = &assessment.AIContext{Sample: &assessment.AISampleContext{AnalysisMode: "trio"}}
			r.Context.DiseaseID = ""
			r.Context.Inheritance = "unknown"
			r.Context.VariantOrigin = "unknown"
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
			if j.Status != jobs.StatusSucceeded || result.ResultKind != "experimental" || result.ClinicalUse != "forbidden" || result.RuleEngineResult != nil || result.SettlementEligible != paid {
				t.Fatal("SVC commercial boundary changed", j.Status, result.FailureCode, result.SettlementEligible)
			}
			for _, cap := range s.descriptors() {
				if cap.ID == engine.EngineSVCv4 && (cap.Enabled || !cap.AIAvailable || cap.BillingAvailable != paid) {
					t.Fatal("SVC capability mismatch")
				}
			}
		})
	}
}
