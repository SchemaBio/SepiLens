package svcv4

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestLockedUpstreamParity(t *testing.T) {
	b, e := os.ReadFile("testdata/upstream-parity.json")
	if e != nil {
		t.Fatal(e)
	}
	var data struct {
		Kind    string `json:"kind"`
		Commit  string `json:"commit"`
		Vectors []struct {
			Difference string `json:"intentional_difference"`
			Exact      *struct {
				Parent string           `json:"parent_code"`
				Sub    map[string]Point `json:"sub_code_points"`
				Held   map[string]Point `json:"held_combined"`
				Total  *Point           `json:"parent_total"`
			} `json:"expected_exact"`
			Workflow   string      `json:"workflow"`
			MOI        string      `json:"moi"`
			Assessment *Impact     `json:"assessment"`
			Population *Population `json:"population"`
			Case       Case        `json:"case"`
			Expected   struct {
				Parent string           `json:"parent_code"`
				Sub    map[string]Point `json:"sub_code_points"`
				Held   map[string]Point `json:"held_combined"`
				Total  *Point           `json:"parent_total"`
			} `json:"expected"`
		} `json:"vectors"`
	}
	if e = json.Unmarshal(b, &data); e != nil {
		t.Fatal(e)
	}
	if data.Commit != Commit || data.Kind != "upstream_reference_parity_not_clinical_goldens" {
		t.Fatal("unlocked parity source")
	}
	for i, v := range data.Vectors {
		if v.Exact != nil {
			if v.Difference != "exact_decimal_POP_FRQ_15x" || v.Workflow != "population" || v.Population.FAF != "0.00015" || v.Population.DAFT != "0.00001" {
				t.Fatal("unexpected parity waiver")
			}
			v.Expected = *v.Exact
		}
		var r FamilyResult
		switch v.Workflow {
		case "population":
			r = scorePopulation(v.Population, v.MOI)
		case "clinical":
			r = scoreClinical([]Case{v.Case}, nil, v.MOI, map[string]Point{"POP_FRQ": 0})
		default:
			r = scoreImpact(v.Assessment, "STRONG")
		}
		if r.ParentCode != v.Expected.Parent || !reflect.DeepEqual(r.Total, v.Expected.Total) || !reflect.DeepEqual(r.SubCodePoints, v.Expected.Sub) || !reflect.DeepEqual(r.Held, v.Expected.Held) {
			t.Errorf("vector %d %s: got %+v; expected %+v", i, v.Workflow, r, v.Expected)
		}
	}
}
func TestBandsExactAndNoData(t *testing.T) {
	for _, v := range []struct {
		p        Point
		cat, sub string
	}{{pt(-4), "benign", ""}, {Point(-3999999), "likely_benign", ""}, {pt(-1), "likely_benign", ""}, {Point(-999999), "uncertain_significance", "VUS-low"}, {Point(1999999), "uncertain_significance", "VUS-low"}, {pt(2), "uncertain_significance", "VUS-mid"}, {pt(4), "uncertain_significance", "VUS-high"}, {pt(6), "likely_pathogenic", ""}, {pt(10), "pathogenic", ""}, {pt(50), "pathogenic", ""}} {
		c, s := Classify(v.p)
		if c != v.cat || s != v.sub {
			t.Fatal(v, c, s)
		}
	}
	r, e := Evaluate(&DraftAssessment{MOI: "AD"})
	if e != nil || r.Total != nil || r.Classification != "insufficient_data" || r.Authoritative || r.InputsVerified || r.ClinicalUse != "forbidden" {
		t.Fatal(r, e)
	}
	r, e = Evaluate(&DraftAssessment{MOI: "AD", Population: &Population{FAF: "0", DAFT: "0.001"}})
	if e != nil || r.Total == nil || *r.Total != 0 {
		t.Fatal("scored zero lost", r, e)
	}
}
func TestValidateAndNoFallback(t *testing.T) {
	for _, s := range []string{`{"moi":"AD","unexpected":1}`, `{"moi":"AD"}{}`, `{"moi":"AD","population":{"faf":"1e999999999"}}`, `{"moi":"AD","variant_impact":{"workflow":"noncoding"}}`, `{"moi":"AD","cases":[{"id":"x","family_id":"f"},{"id":"y","family_id":"f"}]}`, `{"moi":"AD","variant_impact":{"workflow":"missense_amino_acid","initial_points":"NaN"}}`} {
		if _, e := Decode([]byte(s)); e == nil {
			t.Fatal("accepted invalid draft input", s)
		}
	}
	if _, e := Decode([]byte(`{"moi":"AD","upstream_commit":"unlocked"}`)); e == nil {
		t.Fatal("accepted unknown version")
	}
}
func TestMissensePreservesBothPaths(t *testing.T) {
	a := &DraftAssessment{MOI: "AD", GeneDiseaseValidity: "STRONG", Impact: &Impact{Workflow: "missense", AminoAcid: &Impact{Workflow: "missense_amino_acid", Initial: ptr(pt(3)), Exon: "ALL"}, Splice: &Impact{Workflow: "missense_splice", Outcome: "NMD_PREDICTED", Initial: ptr(pt(3)), Mechanism: "ESTABLISHED", Exon: "ALL"}}}
	r, e := Evaluate(a)
	if e != nil {
		t.Fatal(e)
	}
	f := r.Families[len(r.Families)-1]
	if f.SelectedPath != "AMINO_ACID" || len(f.ComparedPaths) != 2 {
		t.Fatal("positive tie lost", f)
	}
	a.Impact.Splice.SpliceAssay = ptr(pt(2))
	r, e = Evaluate(a)
	if e != nil {
		t.Fatal(e)
	}
	f = r.Families[len(r.Families)-1]
	if f.SelectedPath != "SPLICE" || f.Total == nil || *f.Total != pt(5) {
		t.Fatal(f)
	}
}
func TestDeterministicAndClinicalGateSeparate(t *testing.T) {
	a := &DraftAssessment{MOI: "AD", Population: &Population{FAF: "0.00015", DAFT: "0.00001"}}
	first, _ := Evaluate(a)
	b, _ := json.Marshal(first)
	for i := 0; i < 20; i++ {
		r, e := Evaluate(a)
		again, _ := json.Marshal(r)
		if e != nil || string(b) != string(again) {
			t.Fatal("unstable reference result")
		}
	}
	if !strings.Contains(string(b), `"authoritative":false`) {
		t.Fatal("authoritative reference")
	}
}
