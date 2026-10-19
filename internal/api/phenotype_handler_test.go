package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pzweuj/sepilens/internal/annotation"
	"github.com/pzweuj/sepilens/internal/config"
	"github.com/pzweuj/sepilens/internal/hpo"
	"github.com/pzweuj/sepilens/internal/phenotype"
)

func TestPhenotypeEndpointVerifiedAndUnverified(t *testing.T) {
	terms := map[string]hpo.Term{"HP:0000001": {ID: "HP:0000001"}}
	for n := 2; n <= 4; n++ {
		id := fmt.Sprintf("HP:%07d", n)
		terms[id] = hpo.Term{ID: id, Parents: []string{"HP:0000001"}}
	}
	graph, e := hpo.NewGraph(terms)
	if e != nil {
		t.Fatal(e)
	}
	rows := []phenotype.Association{{DiseaseID: "OMIM:1", HPO: "HP:0000002", Frequency: .8, ExplicitFrequency: true}, {DiseaseID: "OMIM:1", HPO: "HP:0000003", Frequency: .8, ExplicitFrequency: true}, {DiseaseID: "OMIM:2", HPO: "HP:0000004", Frequency: .8, ExplicitFrequency: true}}
	for n := 3; n < 35; n++ {
		rows = append(rows, phenotype.Association{DiseaseID: fmt.Sprint("OMIM:", n), HPO: "HP:0000004", Frequency: .8, ExplicitFrequency: true})
	}
	dataset, e := phenotype.NewDataset(graph, rows, "synthetic")
	if e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(t.TempDir(), "gencc.db")
	assets := []annotation.AssetMetadata{{Kind: "gencc", Version: "synthetic", LicenseConfirmed: true, Complete: true, RetrievedAt: time.Now().UTC().Format(time.RFC3339)}}
	if e = annotation.BuildIndex(context.Background(), p, "synthetic", assets, func(emit func(annotation.IndexedRecord) error) error {
		for _, id := range []string{"OMIM:1", "OMIM:2"} {
			b, _ := json.Marshal(annotation.GenCC{Gene: "G", DiseaseID: id, MOI: "AD", Classification: "Strong"})
			if e := emit(annotation.IndexedRecord{Kind: "gencc", Query: annotation.Query{Gene: "G", DiseaseID: id, MOI: "AD"}, Data: b}); e != nil {
				return e
			}
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	index, e := annotation.OpenIndex(p, "synthetic")
	if e != nil {
		t.Fatal(e)
	}
	defer index.Close()
	cfg := config.Config{AuthMode: "required", Principals: map[string]config.Principal{"review": {Tenant: "T", Actor: "reviewer", Role: "reviewer"}, "submit": {Tenant: "T", Actor: "submitter", Role: "submitter"}}}
	s := NewServer(cfg, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.SetAnnotationIndex(index)
	s.SetPhenotypeDataset(dataset)
	for _, token := range []string{"review", "submit"} {
		r := httptest.NewRequest("POST", "/v1/phenotype/match", strings.NewReader(`{"gene":"G","disease_id":"OMIM:1","inheritance":"AD","phenotype":{"source":"clinician_verified","hpo_terms":[{"id":"HP:0000002","polarity":"present"},{"id":"HP:0000003","polarity":"present"}]}}`))
		r.Header.Set("X-API-Key", token)
		w := httptest.NewRecorder()
		s.Router().ServeHTTP(w, r)
		var result PhenotypeAssessment
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil {
			t.Fatal(w.Code, w.Body.String())
		}
		if token == "review" && result.PP4.Status != "met" {
			t.Fatal(result)
		}
		if token == "submit" && result.PP4.Status == "met" {
			t.Fatal("unconfirmed phenotype qualified PP4")
		}
		if strings.Contains(w.Body.String(), "classification") {
			t.Fatal("match endpoint classified variant")
		}
	}
}
