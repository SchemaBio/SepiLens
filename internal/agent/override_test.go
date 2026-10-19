package agent

import (
	"testing"

	"github.com/pzweuj/sepilens/internal/evidence"
	a "github.com/pzweuj/sepilens/internal/rules/acmg2015"
)

func TestSixChangesAndOmissions(t *testing.T) {
	sources := []evidence.Source{{ID: "S", ValidationStatus: evidence.ValidationVerified, IdentifierValid: true, RecordExists: true, ObjectMatched: true, MaterialVerified: true}}
	facts := map[string]bool{}
	var claimed []string
	for _, f := range a.RequiredFacts[a.CodePS3] {
		facts[f] = true
		claimed = append(claimed, f)
	}
	for _, change := range []string{"confirmed", "upgraded_from_insufficient_data", "downgraded", "negated", "strength_changed", "added"} {
		t.Run(change, func(t *testing.T) {
			base := a.Candidate{Code: a.CodePS3, Status: "met", Strength: "Strong", Direction: a.DirectionPathogenic, Facts: facts, SourceIDs: []string{"S"}}
			p := Criterion{Code: base.Code, Status: base.Status, Strength: base.Strength, Change: change, Reason: "synthetic verified basis", Citations: []string{"S"}, Facts: claimed}
			local := a.Derivation{Candidates: []a.Candidate{base}}
			switch change {
			case "upgraded_from_insufficient_data":
				local.Candidates[0].Status = "insufficient_data"
			case "downgraded":
				p.Status = "insufficient_data"
			case "negated":
				p.Status = "not_met"
			case "strength_changed":
				p.Strength = "Supporting"
			case "added":
				local.Candidates = nil
			}
			merged, audit := Merge(local, Assessment{Criteria: []Criterion{p}}, sources, MergeOptions{Facts: map[a.CriterionCode]map[string]bool{a.CodePS3: facts}})
			if len(audit) != 1 || !audit[0].Accepted || len(merged.Candidates) != 1 || merged.Candidates[0].Status != p.Status {
				t.Fatal(merged, audit)
			}
		})
	}
	base := a.Derivation{Candidates: []a.Candidate{{Code: a.CodePP3, Status: "met", Strength: "Supporting"}}}
	merged, audit := Merge(base, Assessment{}, nil, MergeOptions{})
	if len(merged.Candidates) != 1 || audit[0].Reason != "ai_omitted" {
		t.Fatal(merged, audit)
	}
}
func TestUnverifiedFactsAndForgedCitationCannotOverride(t *testing.T) {
	local := a.Derivation{Candidates: []a.Candidate{{Code: a.CodePS3, Status: "insufficient_data", Strength: "Strong"}}}
	p := Criterion{Code: a.CodePS3, Status: "met", Strength: "Strong", Change: "upgraded_from_insufficient_data", Reason: "claimed study", Citations: []string{"fabricated"}}
	for _, f := range a.RequiredFacts[a.CodePS3] {
		p.Facts = append(p.Facts, f)
	}
	for _, sources := range [][]evidence.Source{nil, {{ID: "fabricated", ValidationStatus: evidence.ValidationVerified, IdentifierValid: true, RecordExists: true, ObjectMatched: true, MaterialVerified: true}}} {
		m, audit := Merge(local, Assessment{Criteria: []Criterion{p}}, sources, MergeOptions{})
		if audit[0].Accepted || m.Candidates[0].Status != "insufficient_data" {
			t.Fatal(m, audit)
		}
	}
}
func TestStrictOutputParser(t *testing.T) {
	for _, raw := range []string{`{"criteria":[],"unknown":true}`, `{"criteria":[{"code":"FAKE"}]}`, `{"criteria":[]} {}`, `{"criteria":[{"code":"PP3","status":"met","strength":"Supporting","change":"confirmed"},{"code":"PP3","status":"met","strength":"Supporting","change":"confirmed"}]}`} {
		if _, e := Parse(raw); e == nil {
			t.Fatal("invalid output accepted", raw)
		}
	}
}
