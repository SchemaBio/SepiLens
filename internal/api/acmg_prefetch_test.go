package api

import (
	"context"
	"encoding/json"
	"github.com/pzweuj/sepilens/internal/assessment"
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

type exactSequenceFixture struct{ calls atomic.Int32 }

func (*exactSequenceFixture) Name() string   { return "pubmed" }
func (*exactSequenceFixture) IsOnline() bool { return true }
func (f *exactSequenceFixture) Retrieve(_ context.Context, q retrieval.Query) ([]evidence.Source, error) {
	f.calls.Add(1)
	if q.Gene != "MYH3" || q.HGVS != "c.2015G>A" || q.ProteinHGVS != "p.Arg672His" {
		return nil, context.Canceled
	}
	return []evidence.Source{{ID: "synthetic-public-marker", Type: evidence.SourceTypePubMed, Title: "Synthetic retrieval marker", RecordExists: true, ObjectMatched: true}}, nil
}
func TestACMGImportedContextRetrievesExactVariantBeforeModelWithoutToolCall(t *testing.T) {
	s := newV2Server(t)
	s.cfg.AIEvidenceChain = false
	s.cfg.AICandidateEngines = map[string]bool{"acmg_amp_2015": true}
	search := &exactSequenceFixture{}
	s.SetSearchCoordinator(retrieval.NewSearchCoordinator(retrieval.CoordinatorConfig{}, []retrieval.Retriever{search}))
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		raw, _ := json.Marshal(body["messages"])
		if search.calls.Load() != 1 || !strings.Contains(string(raw), "Synthetic retrieval marker") {
			t.Error("model ran before exact retrieval")
		}
		content := `{"clinical_summary":"Synthetic context and missing evidence","criteria":[{"code":"PM2","status":"insufficient_data","strength":"Moderate","score_hundredths":null,"points":null,"source_ids":[],"facts_confirmed":[],"change":"confirmed","override_reason":"coverage missing"}],"classification":"insufficient_data"}`
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}, "finish_reason": "stop"}}})
	}))
	defer model.Close()
	if _, e := s.profiles.Save(context.Background(), "T", "prefetch", responses.Config{BaseURL: model.URL, APIKey: "synthetic", Model: "synthetic", Protocol: llm.ProtocolOpenAIChat}); e != nil {
		t.Fatal(e)
	}
	r := v2Fixture("acmg_amp_2015")
	r.Mode = "ai_assisted"
	r.Options.ProviderProfileID = "prefetch"
	r.Package.Sources = nil
	r.Package.AIContext = &assessment.AIContext{Sample: &assessment.AISampleContext{AnalysisMode: "single"}}
	r.Variant.Gene = "MYH3"
	r.Variant.HGVSc = "c.2015G>A"
	r.Variant.HGVSp = "p.Arg672His"
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
	if j.Status != jobs.StatusSucceeded {
		t.Fatal(j.Status)
	}
}
