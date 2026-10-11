package svcv4

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestExportBindsActualImplementation(t *testing.T) {
	h := sha256.Sum256(RulePackage())
	if hex.EncodeToString(h[:]) != RulesetHash() {
		t.Fatal("export and runtime rule hash differ")
	}
}

func TestLocusExactBoundsAndCap(t *testing.T) {
	for _, v := range []struct {
		yield string
		want  Point
	}{{"32.999999", 0}, {"33", pt(1)}, {"50", pt(1)}, {"50.000001", pt(2)}, {"68", pt(3)}, {"81.999999", pt(3)}, {"82", pt(4)}} {
		r := scoreLocus(Case{Testing: &Testing{DiagnosticYield: v.yield}}, "AD")
		if r.Total == nil || *r.Total != v.want {
			t.Fatal(v, r)
		}
	}
	r := scoreLocus(Case{Testing: &Testing{DiagnosticYield: "90"}, Relatives: []Relative{{Affected: "TRUE", Exists: "TRUE", Zygosity: "HET"}}}, "AD")
	if r.Total == nil || *r.Total != pt(4) || r.Held["raw_sum"] != pt(5) {
		t.Fatal("LOC cap", r)
	}
	r = scoreLocus(Case{Relatives: []Relative{{Affected: "FALSE", Exists: "FALSE"}}}, "AR")
	if r.Total == nil || *r.Total != Point(400000) {
		t.Fatal("AR fractional segregation", r)
	}
}

func TestClinicalPopulationGateAndCCSExclusion(t *testing.T) {
	c := Case{Phenotype: "SPECIFIC", Zygosity: "HET"}
	r := scoreClinical([]Case{c}, nil, "AD", nil)
	if _, ok := r.SubCodePoints["CLN_AFF"]; ok {
		t.Fatal("missing population gate awards AFF", r)
	}
	r = scoreClinical([]Case{c}, nil, "AD", map[string]Point{"POP_FRQ": pt(-3)})
	if _, ok := r.SubCodePoints["CLN_AFF"]; ok {
		t.Fatal("population exclusion ignored", r)
	}
	ccs := &CaseControl{OddsRatio: "6", CILower: "2", CIUpper: "10", VariantCount: 5, CohortSize: 100, ControlsMatched: true}
	r = scoreClinical([]Case{c}, ccs, "AD", map[string]Point{"POP_FRQ": 0})
	if r.Total == nil || *r.Total != pt(4) {
		t.Fatal(r)
	}
	for _, code := range []string{"CLN_AFF", "CLN_ALT", "CLN_UAF"} {
		if _, ok := r.SubCodePoints[code]; ok {
			t.Fatal("CCS correlated evidence retained", code, r)
		}
	}
	ccs.CILower = "1"
	if p := scoreCCS(ccs); p == nil || *p != 0 {
		t.Fatal("CI includes one", p)
	}
	ccs.VariantCount = 4
	if scoreCCS(ccs) != nil {
		t.Fatal("ineligible cohort scored")
	}
}
