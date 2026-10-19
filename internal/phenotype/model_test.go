package phenotype

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/pzweuj/sepilens/internal/hpo"
)

func fixture(t *testing.T) *Dataset {
	t.Helper()
	g, e := hpo.NewGraph(map[string]hpo.Term{
		"HP:0000001": {ID: "HP:0000001"}, "HP:0000002": {ID: "HP:0000002", Parents: []string{"HP:0000001"}}, "HP:0000003": {ID: "HP:0000003", Parents: []string{"HP:0000002"}}, "HP:0000004": {ID: "HP:0000004", Parents: []string{"HP:0000002"}}, "HP:0000005": {ID: "HP:0000005", Parents: []string{"HP:0000001"}}, "HP:0000006": {ID: "HP:0000006", Parents: []string{"HP:0000005"}},
	})
	if e != nil {
		t.Fatal(e)
	}
	rows := []Association{{DiseaseID: "target", HPO: "HP:0000003", Frequency: .8, ExplicitFrequency: true}, {DiseaseID: "parent", HPO: "HP:0000002", Frequency: .5, ExplicitFrequency: true}, {DiseaseID: "excluded", HPO: "HP:0000003", Excluded: true}, {DiseaseID: "missing-frequency", HPO: "HP:0000003", Frequency: .5}}
	for n := 0; n < 30; n++ {
		rows = append(rows, Association{DiseaseID: fmt.Sprint("background-", n), HPO: "HP:0000006", Frequency: .5, ExplicitFrequency: true})
	}
	d, e := NewDataset(g, rows, "synthetic")
	if e != nil {
		t.Fatal(e)
	}
	return d
}
func TestSixRelationsAndAbsent(t *testing.T) {
	d := fixture(t)
	for _, tc := range []struct{ id, disease, want string }{{"HP:0000003", "target", "exact"}, {"HP:0000002", "target", "ancestor"}, {"HP:0000003", "parent", "descendant"}, {"HP:0000004", "target", "common_ancestor"}, {"HP:0000005", "target", "unrelated"}, {"HP:0000003", "excluded", "excluded"}} {
		m := d.match(Observation{ID: tc.id, Polarity: "present"}, tc.disease)
		if m.Relation != tc.want || math.IsNaN(m.LR) {
			t.Fatal(tc, m)
		}
	}
	m := d.match(Observation{ID: "HP:0000003", Polarity: "absent"}, "target")
	if !m.Conflict || m.Ignored {
		t.Fatal(m)
	}
	m = d.match(Observation{ID: "HP:0000003", Polarity: "absent"}, "missing-frequency")
	if !m.Ignored || m.LR != 1 {
		t.Fatal("unspecified absence affected score", m)
	}
	for _, p := range []string{"uncertain", "family_history", "inferred"} {
		if !d.match(Observation{ID: "HP:0000003", Polarity: p}, "target").Ignored {
			t.Fatal(p)
		}
	}
}
func TestRankingTieSingleAndUnknown(t *testing.T) {
	d := fixture(t)
	in := Input{Terms: []Observation{{ID: "HP:0000003", Polarity: "present"}}}
	r, e := d.Match(in, []string{"target"})
	if e != nil || len(r.Scores) != 1 || r.Scores[0].Margin != nil {
		t.Fatal(r, e)
	}
	d.Diseases["tie"] = d.Diseases["target"]
	r, e = d.Match(in, []string{"target", "tie"})
	if e != nil || r.Scores[0].Rank != 1 || r.Scores[1].Rank != 1 || *r.Scores[0].Margin != 1 {
		t.Fatal(r, e)
	}
	if _, e = d.Match(Input{Terms: []Observation{{ID: "HP:9999999"}}}, []string{"target"}); e == nil {
		t.Fatal("unknown HPO accepted")
	}
	if _, e = d.Match(Input{Terms: []Observation{{ID: "HP:0000003"}, {ID: "HP:0000003"}}}, []string{"target"}); e == nil {
		t.Fatal("duplicate accepted")
	}
}
func TestHPOAFrequencyAndParser(t *testing.T) {
	for _, s := range []string{"1/2", "50%", "HP:0040282", "0.5"} {
		f, explicit, e := Frequency(s)
		if e != nil || f != .5 || !explicit {
			t.Fatal(s, f, e)
		}
	}
	for _, s := range []string{"1/0", "NaN", "Inf", "-1", "200%", "bogus"} {
		if _, _, e := Frequency(s); e == nil {
			t.Fatal(s)
		}
	}
	_, explicit, e := Frequency("")
	if e != nil || explicit {
		t.Fatal("missing frequency not tracked")
	}
	rows, e := ParseHPOA(strings.NewReader("#description\n#database_id\thpo_id\tqualifier\tfrequency\nOMIM:1\tHP:0000003\tNOT\t1/2\n"))
	if e != nil || len(rows) != 1 || !rows[0].Excluded {
		t.Fatal(rows, e)
	}
}
func TestPP4EachGateFails(t *testing.T) {
	margin := 3.
	r := MatchResult{Scores: []DiseaseScore{{DiseaseID: "target", LogLR: math.Log(100), LR: 100, Rank: 1, Margin: &margin, SpecificTerms: 2}}}
	c := PP4Context{ClinicalVerified: true, GenCCStrong: true, SingleCauseConfirmed: true, MOIConsistent: true, InputsVerified: true}
	if PP4(r, "target", c).Status != "met" {
		t.Fatal("positive synthetic gates failed")
	}
	for _, name := range []string{"clinician", "specific", "lr", "rank", "margin", "gencc", "single", "absent", "competition", "refuted"} {
		t.Run(name, func(t *testing.T) {
			cc := c
			rr := r
			rr.Scores = append([]DiseaseScore(nil), r.Scores...)
			switch name {
			case "clinician":
				cc.ClinicalVerified = false
			case "specific":
				rr.Scores[0].SpecificTerms = 1
			case "lr":
				rr.Scores[0].LogLR = 0
			case "rank":
				rr.Scores[0].Rank = 2
			case "margin":
				rr.Scores[0].Margin = nil
			case "gencc":
				cc.GenCCStrong = false
			case "single":
				cc.SingleCauseConfirmed = false
			case "absent":
				rr.Scores[0].Terms = []TermMatch{{Conflict: true}}
			case "competition":
				m := 1.
				rr.Scores[0].Margin = &m
			case "refuted":
				cc.RefutedOrDisputed = true
			}
			if p := PP4(rr, "target", cc); p.Status == "met" || len(p.Missing) == 0 {
				t.Fatal(p)
			}
		})
	}
}

func TestAggregationMonotonicityAndDeterminism(t *testing.T) {
	d := fixture(t)
	d.Diseases["target"] = append(d.Diseases["target"], Association{DiseaseID: "target", HPO: "HP:0000004", Frequency: .8, ExplicitFrequency: true})
	positive := Input{Source: "clinician_verified", Terms: []Observation{{ID: "HP:0000003", Polarity: "present"}}}
	a, e := d.Match(positive, []string{"target", "parent"})
	if e != nil {
		t.Fatal(e)
	}
	positive.Terms = append(positive.Terms, Observation{ID: "HP:0000004", Polarity: "present"})
	b, e := d.Match(positive, []string{"parent", "target"})
	if e != nil || b.Scores[0].LogLR < a.Scores[0].LogLR {
		t.Fatal(a, b, e)
	}
	negative := positive
	negative.Terms = []Observation{{ID: "HP:0000003", Polarity: "present"}, {ID: "HP:0000004", Polarity: "absent"}}
	c, e := d.Match(negative, []string{"target"})
	if e != nil || c.Scores[0].LogLR > a.Scores[0].LogLR {
		t.Fatal("contradiction increased support", a, c, e)
	}
	positive.Terms[0], positive.Terms[1] = positive.Terms[1], positive.Terms[0]
	again, e := d.Match(positive, []string{"target", "parent"})
	if e != nil || again.Scores[0].LogLR != b.Scores[0].LogLR {
		t.Fatal("input order changed result", b, again, e)
	}
}
