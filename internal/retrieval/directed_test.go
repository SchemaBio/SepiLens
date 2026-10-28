package retrieval

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDirectedOfficialQuerySelectsDatabaseAndKeepsGainLossTermsSeparate(t *testing.T) {
	for _, kind := range []string{"cnv_loss", "cnv_gain"} {
		t.Run(kind, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				term := r.URL.Query().Get("term")
				if r.URL.Query().Get("db") != "pubmed" {
					t.Error("unrequested database called")
				}
				if kind == "cnv_gain" && (!strings.Contains(term, "triplosensitivity") || strings.Contains(term, "haploinsufficiency")) {
					t.Error("gain borrowed loss query")
				}
				if kind == "cnv_loss" && (!strings.Contains(term, "haploinsufficiency") || strings.Contains(term, "triplosensitivity")) {
					t.Error("loss borrowed gain query")
				}
				fmt.Fprint(w, `{"esearchresult":{"idlist":[]}}`)
			}))
			defer server.Close()
			coordinator := NewSearchCoordinator(CoordinatorConfig{}, []Retriever{&OfficialRetriever{Database: "pubmed", HTTP: server.Client(), BaseURL: server.URL}, &OfficialRetriever{Database: "clinvar", HTTP: server.Client(), BaseURL: server.URL}})
			budget := &Budget{Max: 1}
			_, sources, err := coordinator.Coordinate(WithBudget(context.Background(), budget), Query{Assembly: "GRCh38", Chrom: "1", Start: 1, End: 100, VariantType: kind, Purpose: "dosage_sensitivity", Databases: []string{"pubmed"}, PreserveRecords: true})
			if err != nil || len(sources) != 0 || calls.Load() != 1 || budget.Count() != 1 {
				t.Fatal("budget or database selection failed", err, calls.Load())
			}
		})
	}
}
func TestCancelledBudgetDoesNotConsumeRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	budget := &Budget{Max: 1}
	if budget.Take(ctx) == nil || budget.Count() != 0 {
		t.Fatal("cancelled budget consumed a request")
	}
}
