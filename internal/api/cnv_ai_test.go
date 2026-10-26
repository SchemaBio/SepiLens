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

type cnvSyntheticRetriever struct {
	calls *atomic.Int32
	fail  bool
}

func (c cnvSyntheticRetriever) Name() string   { return "synthetic" }
func (c cnvSyntheticRetriever) IsOnline() bool { return true }
func (c cnvSyntheticRetriever) Retrieve(ctx context.Context, q retrieval.Query) ([]evidence.Source, error) {
	c.calls.Add(1)
	if q.VariantType != "cnv_gain" && q.VariantType != "cnv_loss" {
		panic("wrong CNV query")
	}
	return nil, nil
}
func TestCNVAutomaticContextPrefetchAndBilling(t *testing.T) {
	for _, id := range []string{"cnv_gain_2020", "cnv_loss_2020"} {
		for _, online := range []bool{true, false} {
			t.Run(id+map[bool]string{true: "online", false: "unavailable"}[online], func(t *testing.T) {
				s := newV2Server(t)
				s.approvals = map[engine.EngineID]engine.Approval{}
				s.cfg.AICandidateEngines = map[string]bool{id: true}
				s.cfg.BillCNVAI = true
				var searches atomic.Int32
				if online {
					s.coordinator = retrieval.NewSearchCoordinator(retrieval.CoordinatorConfig{}, []retrieval.Retriever{cnvSyntheticRetriever{calls: &searches}})
				} else {
					s.coordinator = nil
				}
				model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var body map[string]any
					_ = json.NewDecoder(r.Body).Decode(&body)
					b, _ := json.Marshal(body)
					if !strings.Contains(string(b), "cnv_rubric") || !strings.Contains(string(b), "retrieved_literature") {
						t.Error("CNV context or prefetch absent")
					}
					if online && searches.Load() == 0 {
						t.Error("model ran before retrieval")
					}
					content := `{"clinical_summary":"Synthetic CNV context organized; evidence missing.","cnv_context":{"disease_id":"","inheritance":"unknown","rationale":"Missing evidence","source_ids":[]},"criteria":[{"code":"2A","status":"insufficient_data","strength":"","score_hundredths":null,"points":null,"source_ids":[],"facts_confirmed":[],"change":"confirmed","override_reason":"dosage evidence unavailable"}],"classification":"insufficient_data"}`
					_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}, "finish_reason": "stop"}}})
				}))
				defer model.Close()
				if _, e := s.profiles.Save(context.Background(), "T", "mock", responses.Config{BaseURL: model.URL, APIKey: "synthetic", Model: "synthetic", Protocol: llm.ProtocolOpenAIChat}); e != nil {
					t.Fatal(e)
				}
				r := v2Fixture(id)
				r.Mode = "ai_assisted"
				r.Package.Sources = nil
				r.Package.Criteria = nil
				r.Package.AIContext = &assessment.AIContext{}
				r.Context.DiseaseID = ""
				r.Context.Inheritance = ""
				r.Context.SampleType = ""
				r.Context.VariantOrigin = ""
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
				if result.AgentProposal.(map[string]any)["ai_assessment"].(map[string]any)["cnv_context"] == nil {
					t.Fatal("CNV inferred context lost in delivery")
				}
				if j.Status != jobs.StatusSucceeded || result.SettlementEligible != online || result.ClinicalUse != "forbidden" || result.ResultKind != "evidence_only" {
					t.Fatal(j.Status, result.FailureCode, result.SettlementEligible)
				}
				for _, c := range s.descriptors() {
					if string(c.ID) == id && (!c.AIAvailable || !c.BillingAvailable || c.Enabled) {
						t.Fatal(c)
					}
				}
			})
		}
	}
}
func TestCNVProposalRejectsWrongBoundsSourcesAndGroups(t *testing.T) {
	n := int64(100)
	c := assessmentChange{Criterion: assessment.Criterion{Code: "2A", Status: "met", Score: &n, SourceIDs: []string{"s"}, Facts: map[string]bool{"complete_hi_overlap": true}}, OverrideReason: "synthetic"}
	src := []evidence.Source{{ID: "s", IdentifierValid: true, RecordExists: true}}
	p := assessmentProposal{Criteria: []assessmentChange{c}}
	if e := validateCNVProposal("cnv_loss_2020", p, src); e != nil {
		t.Fatal(e)
	}
	n = 101
	if validateCNVProposal("cnv_loss_2020", p, src) == nil {
		t.Fatal("bounds bypass")
	}
	n = 100
	if validateCNVProposal("cnv_loss_2020", p, nil) == nil {
		t.Fatal("citation bypass")
	}
	p.Criteria = append(p.Criteria, c)
	if validateCNVProposal("cnv_loss_2020", p, src) == nil {
		t.Fatal("duplicate accepted")
	}
}

func TestCNVModelDistinguishesZeroCopyFromUnknown(t *testing.T) {
	r := v2Fixture("cnv_loss_2020")
	r.Mode = "ai_assisted"
	r.CNV.CopyNumber = 0
	r.Package.AIContext = &assessment.AIContext{VariantAnnotations: map[string]any{"copyNumber": float64(0)}}
	input := modelAssessmentInput(r, assessment.Result{}, nil)
	if input["cnv"].(map[string]any)["copy_number"] != float64(0) {
		t.Fatal("zero CN lost")
	}
	delete(r.Package.AIContext.VariantAnnotations, "copyNumber")
	input = modelAssessmentInput(r, assessment.Result{}, nil)
	if input["cnv"].(map[string]any)["copy_number"] != nil {
		t.Fatal("missing CN treated as zero")
	}
}
