package api

import (
	"context"
	"encoding/json"
	"github.com/pzweuj/sepilens/internal/assessment"
	"github.com/pzweuj/sepilens/internal/engine"
	"github.com/pzweuj/sepilens/internal/evidence"
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

func svcTestContext(content string, svc bool) string {
	if !svc {
		return content
	}
	var value map[string]any
	_ = json.Unmarshal([]byte(content), &value)
	fields := map[string]any{}
	for _, key := range []string{"disease", "moi", "geneDiseaseValidity", "workflow"} {
		fields[key] = map[string]any{"value": "", "reason": "Synthetic source unavailable", "source_ids": []string{}}
	}
	value["svc_context"] = fields
	raw, _ := json.Marshal(value)
	return string(raw)
}
func mockSVCSearch(s *Server) {
	s.SetSearchCoordinator(retrieval.NewSearchCoordinator(retrieval.CoordinatorConfig{}, nil))
}
func TestSVCContextUsesOnlySuppliedReferences(t *testing.T) {
	r := assessment.Request{EngineID: "svc_v4", Package: assessment.Package{AIContext: &assessment.AIContext{}}}
	p := &svcContextProposal{Disease: svcContextField{Value: "Synthetic D", Reason: "Matched publication", SourceIDs: []string{"src_1"}}, MOI: svcContextField{Value: "AD", Reason: "Annotation", SourceIDs: []string{"annotation"}}}
	sources := []evidence.Source{{ID: "PMID:1", Title: "Synthetic study"}}
	if err := validateSVCContext(p, r, sources, map[string]string{"src_1": "PMID:1"}); err != nil || len(p.Sources) != 1 || p.Disease.SourceIDs[0] != "PMID:1" {
		t.Fatal("provenance lost", err)
	}
	p.Workflow = svcContextField{Value: "missense", Reason: "fake", SourceIDs: []string{"invented"}}
	if validateSVCContext(p, r, sources, nil) == nil {
		t.Fatal("invented citation accepted")
	}
}

type svcSearchFixture struct{ calls atomic.Int32 }

func (*svcSearchFixture) Name() string   { return "synthetic-official" }
func (*svcSearchFixture) IsOnline() bool { return true }
func (f *svcSearchFixture) Retrieve(_ context.Context, q retrieval.Query) ([]evidence.Source, error) {
	f.calls.Add(1)
	return []evidence.Source{{ID: "PMID:123", PMID: "123", Type: evidence.SourceTypePubMed, Title: "Synthetic G disease study", URL: "https://pubmed.ncbi.nlm.nih.gov/123/", Snippet: "Synthetic disease; AD; curated validity MODERATE"}}, nil
}
func TestSVCSearchRunsBeforeModelAndReturnsAutofillWithCitations(t *testing.T) {
	s := newV2Server(t)
	s.approvals = map[engine.EngineID]engine.Approval{}
	search := &svcSearchFixture{}
	s.SetSearchCoordinator(retrieval.NewSearchCoordinator(retrieval.CoordinatorConfig{}, []retrieval.Retriever{search}))
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		raw, _ := json.Marshal(body["messages"])
		if search.calls.Load() != 1 || !strings.Contains(string(raw), "Synthetic G disease study") {
			t.Error("online search was not performed before generation")
		}
		content := svcTestContext(`{"clinical_summary":"Synthetic reviewed SVC context","classification":"VUS","criteria":[]}`, true)
		var value map[string]any
		_ = json.Unmarshal([]byte(content), &value)
		fields := value["svc_context"].(map[string]any)
		for key, v := range map[string]string{"disease": "Synthetic D", "moi": "AD", "geneDiseaseValidity": "MODERATE", "workflow": "missense"} {
			source := "src_1"
			if key == "workflow" {
				source = "annotation"
			}
			fields[key] = map[string]any{"value": v, "reason": "Synthetic supporting evidence", "source_ids": []string{source}}
		}
		b, _ := json.Marshal(value)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(b)}, "finish_reason": "stop"}}})
	}))
	defer model.Close()
	if _, err := s.profiles.Save(context.Background(), "T", "autofill", responses.Config{BaseURL: model.URL, APIKey: "synthetic", Model: "synthetic", Protocol: llm.ProtocolOpenAIChat}); err != nil {
		t.Fatal(err)
	}
	r := v2Fixture("svc_v4")
	r.Mode = "ai_assisted"
	r.Options.AllowExperimental = true
	r.Options.ProviderProfileID = "autofill"
	r.Package.Draft = nil
	r.Package.DraftSourceIDs = nil
	r.Package.Sources = nil
	r.Package.AIContext = &assessment.AIContext{Sample: &assessment.AISampleContext{AnalysisMode: "trio"}}
	r.Context.DiseaseID = ""
	r.Context.Inheritance = "unknown"
	r.Context.VariantOrigin = "unknown"
	w := callV2(s, "POST", "/v2/evaluations", r, "key", "autofill")
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	var a struct {
		ID string `json:"job_id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &a)
	j := waitV2(t, s, a.ID)
	if j.Status != jobs.StatusSucceeded {
		t.Fatal("SVC autofill failed", j.ErrorJSON)
	}
	var result EvaluationResultPayload
	_ = json.Unmarshal([]byte(j.ResultJSON), &result)
	b, _ := json.Marshal(result.AgentProposal)
	var proposal struct {
		AI struct {
			Context svcContextProposal `json:"svc_context"`
		} `json:"ai_assessment"`
	}
	_ = json.Unmarshal(b, &proposal)
	if proposal.AI.Context.Disease.Value != "Synthetic D" || proposal.AI.Context.MOI.Value != "AD" || proposal.AI.Context.GeneDiseaseValidity.Value != "MODERATE" || proposal.AI.Context.Workflow.Value != "missense" || len(proposal.AI.Context.Sources) != 1 {
		t.Fatal("structured context or sources missing")
	}
}
