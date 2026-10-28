package api

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/pzweuj/sepilens/internal/assessment"
	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/retrieval"
	"strings"
	"testing"
)

func TestMissingEvidencePlanIsEngineScopedAndPrioritizesKnownConflicts(t *testing.T) {
	for _, engine := range []string{"acmg_amp_2015", "svc_v4", "cnv_loss_2020", "cnv_gain_2020"} {
		plan := missingEvidencePlan(engine, assessment.Result{EngineID: engine, Status: "conflicting"})
		if len(plan) > 3 || len(plan) == 0 || plan[0].Purpose != "variant_conflict" {
			t.Fatal("conflict not prioritized", engine)
		}
		for _, need := range plan {
			for _, code := range need.Codes {
				if !assessment.ValidCode(engine, code) {
					t.Fatal("foreign engine code", engine, code)
				}
			}
		}
	}
	if missingEvidencePlan("unknown", assessment.Result{}) != nil {
		t.Fatal("unknown engine inherited a plan")
	}
	base := assessment.Result{EngineID: "cnv_loss_2020", Criteria: []assessment.Criterion{{Code: "2A", Status: "met", SourceIDs: []string{"source"}}}}
	if missingEvidencePlan("cnv_gain_2020", base)[0].Supported {
		t.Fatal("loss support crossed into gain")
	}
	if !missingEvidencePlan("cnv_loss_2020", base)[0].Supported {
		t.Fatal("independent baseline support lost")
	}
	acmg := assessment.Result{EngineID: "acmg_amp_2015", Criteria: []assessment.Criterion{{Code: "PVS1", Status: "met", SourceIDs: []string{"source"}}}}
	functional := missingEvidencePlan(acmg.EngineID, acmg)[1]
	if functional.Supported || len(functional.SupportedCodes) != 1 || functional.SupportedCodes[0] != "PVS1" {
		t.Fatal("one supported clause suppressed unresolved functional questions")
	}
}
func TestDirectedRetrievalAdvancesQuestionsAndKeepsAllCitationRecords(t *testing.T) {
	calls := 0
	d := &directedRetrieval{Query: retrieval.Query{Assembly: "GRCh38", Chrom: "1", Pos: 100, Ref: "A", Alt: "G", HGVS: "FIXED:c.1A>G", VariantType: "snv"}, Needs: missingEvidencePlan("acmg_amp_2015", assessment.Result{}), Count: func() int { return calls }}
	d.Fetch = func(_ context.Context, q retrieval.Query) ([]evidence.Source, bool, error) {
		calls++
		if q.Pos != 100 || q.Ref != "A" || q.Alt != "G" || !q.PreserveRecords {
			t.Fatal("evaluation object changed")
		}
		if q.Purpose == "gene_disease_validity" {
			if q.HGVS != "" {
				t.Fatal("gene query was not broadened")
			}
			return nil, true, nil
		}
		return []evidence.Source{{ID: "same-record", Type: evidence.SourceTypePubMed, PMID: "123", DOI: "10.1234/test", Title: "Synthetic article", Snippet: "Synthetic material for " + q.Purpose, CaseIDs: []string{"PRIVATE_CASE"}}}, true, nil
	}
	for i := 0; i < 6; i++ {
		if err := d.Next(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 3 || len(d.Findings) != 3 || d.Findings[2].Status != "not_found" || len(d.Sources) != 2 || d.Sources[0].ID != "same-record" || d.Sources[1].ID == "same-record" {
		t.Fatal("cache or record preservation failed")
	}
	view := opaqueCitations(d.View(), func(id string) string { return "src_opaque" })
	raw, _ := json.Marshal(view)
	if strings.Contains(string(raw), "PRIVATE_CASE") || strings.Contains(string(raw), "same-record") {
		t.Fatal("private case/source identities exposed")
	}
	groups := d.View()["sources"].([]map[string]any)
	if len(groups) != 1 || len(groups[0]["related_records"].([]map[string]any)) != 1 {
		t.Fatal("same paper not grouped")
	}
}
func TestDirectedRetrievalDistinguishesFailureConflictAndUnqueried(t *testing.T) {
	for _, status := range []string{"unavailable", "conflicting", "not_found"} {
		t.Run(status, func(t *testing.T) {
			calls := 0
			d := &directedRetrieval{Needs: missingEvidencePlan("svc_v4", assessment.Result{}), Count: func() int { return calls }}
			d.Fetch = func(context.Context, retrieval.Query) ([]evidence.Source, bool, error) {
				calls++
				if status == "not_found" {
					return nil, true, nil
				}
				source := evidence.Source{ID: "synthetic", PMID: "123"}
				if status == "conflicting" {
					source.ValidationStatus = evidence.ValidationConflict
					return []evidence.Source{source}, true, nil
				}
				return []evidence.Source{source}, false, errors.New("synthetic failure")
			}
			err := d.Next(context.Background())
			if status == "unavailable" && err == nil {
				t.Fatal("fatal error suppressed")
			}
			if len(d.Findings) != 1 || d.Findings[0].Status != status || d.View()["remaining_questions"] != 2 {
				t.Fatal("retrieval states conflated", d.Findings)
			}
			if status != "not_found" && len(d.Sources) != 1 {
				t.Fatal("partial source lost")
			}
		})
	}
}
func TestDirectedRetrievalSkipsSupportedGapsAndStopsOnCancellation(t *testing.T) {
	calls := 0
	base := assessment.Result{EngineID: "cnv_loss_2020", Criteria: []assessment.Criterion{{Code: "2A", Status: "met", SourceIDs: []string{"source"}}}}
	d := &directedRetrieval{Needs: missingEvidencePlan(base.EngineID, base), Count: func() int { return calls }, Fetch: func(context.Context, retrieval.Query) ([]evidence.Source, bool, error) {
		calls++
		return nil, true, nil
	}}
	if err := d.Next(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(d.Findings) != 2 || d.Findings[0].Status != "supported" || len(d.Findings[0].Codes) != 1 || d.Findings[0].Codes[0] != "2A" {
		t.Fatal("known support queried or overclaimed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(d.Next(ctx), context.Canceled) || calls != 1 {
		t.Fatal("cancellation issued a query")
	}
}
