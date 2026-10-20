package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pzweuj/sepilens/internal/assessment"
	"github.com/pzweuj/sepilens/internal/config"
	"github.com/pzweuj/sepilens/internal/engine"
	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/jobs"
	"github.com/pzweuj/sepilens/internal/llm"
	"github.com/pzweuj/sepilens/internal/llm/responses"
	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
	"github.com/pzweuj/sepilens/internal/rules/svcv4"
	"github.com/pzweuj/sepilens/internal/storage/sqlite"
	"github.com/pzweuj/sepilens/internal/validation"
	"github.com/pzweuj/sepilens/internal/variant/cnv"
)

func newV2Server(t *testing.T) *Server {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "v2.db"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{AuthMode: "required", MasterKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", AIMode: "enabled", AllowPrivateEndpoints: true, RetentionDays: 90, Principals: map[string]config.Principal{"key": {Tenant: "T", Actor: "YiJian", Role: "submitter", SubmitEvidence: true}, "review": {Tenant: "T", Actor: "R", Role: "reviewer"}, "other": {Tenant: "other", Actor: "O", Role: "admin"}, "unprivileged": {Tenant: "T", Actor: "U", Role: "submitter"}}, SourcePolicies: map[string]config.SourcePolicy{"clinical": {Versions: []string{"test"}, Actors: []string{"YiJian"}, Assemblies: []string{"GRCh38"}, Clinical: true}}}
	s := NewServer(cfg, jobs.NewSQLiteStore(db), db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.approvals = map[engine.EngineID]engine.Approval{}
	for _, id := range []engine.EngineID{engine.EngineACMG2015, engine.EngineCNVGain2020, engine.EngineCNVLoss2020} {
		content, _, e := validation.Rules(id)
		if e != nil {
			t.Fatal(e)
		}
		s.approvals[id] = engine.Approval{ContentHash: engine.Hash(content)}
	}
	if err = s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close(); db.Close() })
	return s
}
func v2Fixture(id string) assessment.Request {
	r := assessment.Request{SchemaVersion: assessment.SchemaVersion, EngineID: id, Mode: "local", Context: acmg2015.ClinicalContextInput{DiseaseID: "MONDO:1", Inheritance: "AD", VariantOrigin: "germline_confirmed", SampleType: "blood"}, Package: assessment.Package{Sources: []assessment.Source{}}}
	if id == "acmg_amp_2015" || id == "svc_v4" {
		r.Variant = &acmg2015.SNVVariantInput{Assembly: "GRCh38", Chrom: "1", Pos: 100, Ref: "A", Alt: "G", Gene: "G"}
	} else {
		typ := "loss"
		if id == "cnv_gain_2020" {
			typ = "gain"
		}
		r.CNV = &assessment.Region{CNVInput: cnv.CNVInput{Assembly: "GRCh38", Chrom: "1", Start: 1, End: 100, Type: typ}, CoordinateSystem: "1-based-inclusive"}
	}
	if id == "svc_v4" {
		r.Options.AllowExperimental = true
		r.Package.Draft = &svcv4.DraftAssessment{MOI: "AD", GeneDiseaseValidity: "STRONG", Population: &svcv4.Population{FAF: "0.00015", DAFT: "0.00001"}}
		r.Package.Sources = []assessment.Source{{ID: "draft", Dataset: "clinical", Kind: "clinical", Version: "test", RetrievedAt: time.Now().UTC().Format(time.RFC3339), LicenseConfirmed: true, RecordID: "analyst-record", Object: assessment.ObjectFor(r), Clinical: true}}
		r.Package.DraftSourceIDs = []string{"draft"}
	}
	return r
}
func callV2(s *Server, method, path string, body any, key, idem string) *httptest.ResponseRecorder {
	var raw string
	switch v := body.(type) {
	case string:
		raw = v
	case nil:
	default:
		b, _ := json.Marshal(body)
		raw = string(b)
	}
	r := httptest.NewRequest(method, path, strings.NewReader(raw))
	r.Header.Set("Authorization", "Bearer "+key)
	r.Header.Set("Idempotency-Key", idem)
	w := httptest.NewRecorder()
	s.Router().ServeHTTP(w, r)
	return w
}
func TestV2LocalFourEnginesPersistAndReplayWithoutMedicalDatabase(t *testing.T) {
	for _, id := range []string{"acmg_amp_2015", "svc_v4", "cnv_gain_2020", "cnv_loss_2020"} {
		t.Run(id, func(t *testing.T) {
			s := newV2Server(t)
			r := v2Fixture(id)
			w := callV2(s, "POST", "/v2/evaluations", r, "key", "same")
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			var first EvaluationResultPayload
			_ = json.Unmarshal(w.Body.Bytes(), &first)
			if first.AssessmentID == "" || first.LocalBaseline == nil || first.SettlementEligible {
				t.Fatal(first)
			}
			if id == "svc_v4" && (first.RuleEngineResult != nil || first.ClinicalUse != "forbidden") {
				t.Fatal("SVC promoted", first)
			}
			w = callV2(s, "POST", "/v2/evaluations", r, "key", "same")
			var second EvaluationResultPayload
			_ = json.Unmarshal(w.Body.Bytes(), &second)
			if w.Code != 200 || first.AssessmentID != second.AssessmentID {
				t.Fatal("idempotent replay", w.Body.String())
			}
			r.Context.DiseaseID = "MONDO:2"
			for n := range r.Package.Sources {
				r.Package.Sources[n].Object = assessment.ObjectFor(r)
			}
			if w = callV2(s, "POST", "/v2/evaluations", r, "key", "same"); w.Code != 409 {
				t.Fatal("idempotency conflict missing", w.Code)
			}
			if w = callV2(s, "GET", "/v2/jobs/"+first.AssessmentID+"/result", nil, "other", ""); w.Code != 404 {
				t.Fatal("tenant leak")
			}
			if w = callV2(s, "GET", "/v2/jobs/"+first.AssessmentID+"/audit", nil, "review", ""); w.Code != 200 || !strings.Contains(w.Body.String(), first.InputHash) {
				t.Fatal("audit absent", w.Code, w.Body.String())
			}
		})
	}
}
func TestV2InputPermissionsStrictJSONAndLimits(t *testing.T) {
	s := newV2Server(t)
	r := v2Fixture("acmg_amp_2015")
	if w := callV2(s, "POST", "/v2/evaluations", r, "unprivileged", ""); w.Code != 403 {
		t.Fatal(w.Code)
	}
	for _, raw := range []string{`{"schema_version":"bad","schema_version":"sepilens-evaluation-2.0"}`, `{"verified":true}`, `{"mode":"local","mode":"ai_assisted"}`} {
		if w := callV2(s, "POST", "/v2/evaluations", raw, "key", ""); w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	s.cfg.MaxEvidenceBytes = 1024
	if w := callV2(s, "POST", "/v2/evaluations", strings.Repeat(" ", 1025), "key", ""); w.Code != 413 {
		t.Fatal("size", w.Code)
	}
	if w := callV2(s, "POST", "/v1/variants/snv/evaluations", r, "key", ""); w.Code != 404 {
		t.Fatal("legacy route retained")
	}
}

func TestV2RevisionPreservesBaselineClearsApprovalAndRecomputes(t *testing.T) {
	s := newV2Server(t)
	r := v2Fixture("acmg_amp_2015")
	r.Package.Sources = []assessment.Source{{ID: "clinical-PS3", Dataset: "clinical", Kind: "clinical", Version: "test", RetrievedAt: time.Now().UTC().Format(time.RFC3339), LicenseConfirmed: true, RecordID: "private-record", Object: assessment.ObjectFor(r), Clinical: true}}
	facts := map[string]bool{}
	for _, f := range acmg2015.RequiredFacts[acmg2015.CodePS3] {
		facts[f] = true
	}
	r.Package.Criteria = []assessment.Criterion{{Code: "PS3", Status: "met", Strength: "Strong", SourceIDs: []string{"clinical-PS3"}, Facts: facts}}
	w := callV2(s, "POST", "/v2/evaluations", r, "key", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var initial EvaluationResultPayload
	_ = json.Unmarshal(w.Body.Bytes(), &initial)
	path := "/v2/evaluations/" + initial.AssessmentID + "/reviews"
	w = callV2(s, "POST", path, AddReviewRequest{Revision: 1, Decision: "accept_engine_result", Reason: "reviewed"}, "review", "")
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = callV2(s, "GET", "/v2/jobs/"+initial.AssessmentID+"/result", nil, "key", "")
	var approved EvaluationResultPayload
	_ = json.Unmarshal(w.Body.Bytes(), &approved)
	if !approved.SettlementEligible {
		t.Fatal("valid review not eligible", w.Body.String())
	}
	correction := evidence.EvidenceItem{EngineID: r.EngineID, RuleCode: "PS3", Status: evidence.StatusNotMet, SuggestedScore: "Strong", SourceIDs: []string{"clinical-PS3"}, Reason: "professional correction", Facts: facts}
	w = callV2(s, "POST", path, AddReviewRequest{Revision: 1, Decision: "revise_evidence", Reason: "updated applicability", Corrections: []evidence.EvidenceItem{correction}}, "review", "")
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = callV2(s, "GET", "/v2/jobs/"+initial.AssessmentID+"/result", nil, "key", "")
	var revised EvaluationResultPayload
	_ = json.Unmarshal(w.Body.Bytes(), &revised)
	if revised.Revision != 2 || revised.ReviewerDecision != nil || revised.SettlementEligible {
		t.Fatal("old approval survived", w.Body.String())
	}
	b, _ := json.Marshal(revised.RuleEngineResult)
	var effective assessment.Result
	_ = json.Unmarshal(b, &effective)
	for _, c := range effective.Criteria {
		if c.Code == "PS3" && c.Status != "not_met" {
			t.Fatal("correction lost", c)
		}
	}
	if w = callV2(s, "POST", path, AddReviewRequest{Revision: 1, Decision: "accept_engine_result", Reason: "stale"}, "review", ""); w.Code != 409 {
		t.Fatal("stale revision accepted")
	}
	w = callV2(s, "GET", "/v2/jobs/"+initial.AssessmentID+"/audit", nil, "review", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "professional correction") {
		t.Fatal("revision audit absent")
	}
}
func waitV2(t *testing.T, s *Server, id string) jobs.Job {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		j, e := s.store.GetJob(context.Background(), "T", id)
		if e != nil {
			t.Fatal(e)
		}
		if j.Status == jobs.StatusSucceeded || j.Status == jobs.StatusFailed {
			return j
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("worker timeout")
	return jobs.Job{}
}

func TestV2SixChangesCitationsAndOmissions(t *testing.T) {
	for _, change := range []string{"confirmed", "upgraded_from_insufficient_data", "downgraded", "negated", "strength_changed", "added"} {
		t.Run(change, func(t *testing.T) {
			r := v2Fixture("acmg_amp_2015")
			c := assessment.Criterion{Code: "PS3", Status: "met", Strength: "Strong", SourceIDs: []string{"clinical"}, Facts: map[string]bool{}}
			for _, f := range acmg2015.RequiredFacts[acmg2015.CodePS3] {
				c.Facts[f] = true
			}
			r.Package.Sources = []assessment.Source{{ID: "clinical", Clinical: true}}
			sources := []evidence.Source{{ID: "clinical", ValidationStatus: evidence.ValidationVerified, IdentifierValid: true, RecordExists: true, ObjectMatched: true, MaterialVerified: true}}
			prior := c
			switch change {
			case "upgraded_from_insufficient_data":
				prior.Status = "insufficient_data"
				r.Package.Criteria = []assessment.Criterion{c}
			case "downgraded":
				c.Status = "insufficient_data"
				r.Package.Criteria = []assessment.Criterion{c}
			case "negated":
				c.Status = "not_met"
				r.Package.Criteria = []assessment.Criterion{c}
			case "strength_changed":
				c.Strength = "Supporting"
			case "added":
				r.Package.Criteria = []assessment.Criterion{c}
			}
			base := assessment.Result{Criteria: []assessment.Criterion{prior, {Code: "BP4", Status: "insufficient_data"}}}
			if change == "added" {
				base.Criteria = base.Criteria[1:]
			}
			proposal := assessmentProposal{Criteria: []assessmentChange{{Criterion: c, Change: change, OverrideReason: "structured evidence basis"}}}
			_, audit, err := mergeAssessment(r, base, proposal, sources)
			if err != nil || len(audit) != 2 || !audit[0].Accepted || audit[1].Reason != "ai_omitted" {
				t.Fatal(change, audit, err)
			}
			proposal.Criteria[0].SourceIDs = []string{"fabricated"}
			_, audit, err = mergeAssessment(r, base, proposal, sources)
			if err != nil || audit[0].Accepted {
				t.Fatal("forged citation accepted")
			}
			proposal.Criteria = append(proposal.Criteria, proposal.Criteria[0])
			if _, _, err = mergeAssessment(r, base, proposal, sources); err == nil {
				t.Fatal("duplicate clause accepted")
			}
		})
	}
}

func TestV2PublishedExamplesUseActualContract(t *testing.T) {
	s := newV2Server(t)
	raw, err := os.ReadFile("../../docs/examples/v2/source-policies.synthetic.json")
	if err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(raw, &s.cfg.SourcePolicies) != nil {
		t.Fatal("invalid example policies")
	}
	p := s.cfg.Principals["key"]
	p.Actor = "yijian"
	s.cfg.Principals["key"] = p
	for _, name := range []string{"acmg-local.json", "svc-local.json", "cnv-gain-local.json", "cnv-loss-local.json"} {
		t.Run(name, func(t *testing.T) {
			b, e := os.ReadFile("../../docs/examples/v2/" + name)
			if e != nil {
				t.Fatal(e)
			}
			var req assessment.Request
			if json.Unmarshal(b, &req) != nil {
				t.Fatal("invalid example")
			}
			// Example timestamps describe a fixed synthetic release; refresh only the
			// test clock inputs so the interface test does not become a stale-source test.
			now := time.Now().UTC().Format(time.RFC3339)
			for n := range req.Package.Sources {
				req.Package.Sources[n].RetrievedAt = now
			}
			if a := req.Package.Annotations; a != nil && a.AlphaMissense != nil {
				a.AlphaMissense.RetrievedAt = now
			}
			w := callV2(s, "POST", "/v2/evaluations", req, "key", "")
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
			var result EvaluationResultPayload
			_ = json.Unmarshal(w.Body.Bytes(), &result)
			if req.EngineID == "svc_v4" {
				b, _ := json.Marshal(result.AgentProposal)
				if !strings.Contains(string(b), `"total_points":"-6"`) {
					t.Fatal("draft example not scored", string(b))
				}
			} else {
				b, _ := json.Marshal(result.RuleEngineResult)
				var rule assessment.Result
				_ = json.Unmarshal(b, &rule)
				if rule.Status != "calculated" {
					t.Fatal("example evidence not applied", w.Body.String())
				}
				if req.CNV != nil && rule.Classification != "Pathogenic" {
					t.Fatal(rule)
				}
			}
		})
	}
}
func TestV2AllEnginesBothProtocolsMultiRoundAndPartialFailure(t *testing.T) {
	for _, id := range []string{"acmg_amp_2015", "svc_v4", "cnv_gain_2020", "cnv_loss_2020"} {
		for _, protocol := range []llm.Protocol{llm.ProtocolOpenAIResponses, llm.ProtocolOpenAIChat} {
			for _, fail := range []bool{false, true} {
				t.Run(fmt.Sprint(id, "/", protocol, "/failure=", fail), func(t *testing.T) {
					var calls atomic.Int32
					model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						body, _ := io.ReadAll(r.Body)
						if strings.Contains(string(body), "analyst-record") {
							t.Error("private record identifier sent to model")
						}
						n := calls.Add(1)
						if fail {
							w.WriteHeader(500)
							fmt.Fprint(w, "secret patient provider response")
							return
						}
						if n == 1 {
							if protocol == llm.ProtocolOpenAIResponses {
								fmt.Fprint(w, `{"status":"completed","output":[{"type":"function_call","call_id":"fixed","name":"get_annotations","arguments":"{}"}]}`)
							} else {
								fmt.Fprint(w, `{"choices":[{"message":{"tool_calls":[{"id":"fixed","type":"function","function":{"name":"get_annotations","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`)
							}
							return
						}
						content := `{"criteria":[],"classification":"Pathogenic"}`
						if id == "svc_v4" {
							content = `{"criteria":[{"code":"POP","status":"met","strength":"","score_hundredths":null,"points":"-6","source_ids":["src_1"],"facts_confirmed":[],"change":"confirmed","override_reason":"confirm fixed population calculation"}],"classification":"Pathogenic"}`
						}
						if protocol == llm.ProtocolOpenAIResponses {
							_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output": []any{map[string]any{"content": []any{map[string]any{"type": "output_text", "text": content}}}}})
						} else {
							_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}, "finish_reason": "stop"}}})
						}
					}))
					defer model.Close()
					s := newV2Server(t)
					if _, e := s.profiles.Save(context.Background(), "T", "mock", responses.Config{BaseURL: model.URL, APIKey: "synthetic", Model: "synthetic", Protocol: protocol}); e != nil {
						t.Fatal(e)
					}
					r := v2Fixture(id)
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
					if json.Unmarshal([]byte(j.ResultJSON), &result) != nil || result.LocalBaseline == nil {
						t.Fatal("baseline lost", j)
					}
					if fail {
						if j.Status != jobs.StatusFailed || result.FailureCode == "" || result.SettlementEligible {
							t.Fatal("failure not explicit", result)
						}
					} else {
						if j.Status != jobs.StatusSucceeded || calls.Load() != 2 {
							snapshot, _ := s.evidenceStore.GetSnapshot(context.Background(), "T", j.JobID)
							t.Fatalf("multi-round failed: calls=%d error=%s trace=%s", calls.Load(), result.FailureCode, snapshot.Agent)
						}
					}
					if strings.Contains(j.ResultJSON, "secret patient") {
						t.Fatal("provider error leaked")
					}
					if id == "svc_v4" && result.RuleEngineResult != nil {
						t.Fatal("SVC clinical promotion")
					}
				})
			}
		}
	}
}
