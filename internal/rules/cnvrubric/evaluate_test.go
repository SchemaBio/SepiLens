package cnvrubric_test

import (
	g "github.com/pzweuj/sepilens/internal/rules/cnvgain2020"
	l "github.com/pzweuj/sepilens/internal/rules/cnvloss2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvrubric"
	"github.com/pzweuj/sepilens/internal/variant/cnv"
	"testing"
)

func selected(defs []cnvrubric.Definition, code string, score int64, id string) cnvrubric.Selection {
	facts := map[string]bool{}
	for _, d := range defs {
		if d.Code == code {
			for _, f := range d.Facts {
				facts[f] = true
			}
		}
	}
	return cnvrubric.Selection{Code: code, Score: score, Facts: facts, SourceIDs: []string{"source"}, CaseIDs: []string{id}, Verified: true}
}
func TestOfficialCrossCategoryCaps(t *testing.T) {
	for _, engine := range []struct {
		id    string
		rules []byte
	}{{"cnv_loss_2020", l.Rules}, {"cnv_gain_2020", g.Rules}} {
		t.Run(engine.id, func(t *testing.T) {
			d := cnvrubric.Parse(engine.rules)
			// Independent expected totals from Riggs 2020 Supplement 1: de novo 0.90, segregation 0.45.
			cases := []struct {
				e    []cnvrubric.Selection
				want int64
			}{{[]cnvrubric.Selection{selected(d, "4A", 45, "a"), selected(d, "4B", 45, "b"), selected(d, "4C", 30, "c")}, 90}, {[]cnvrubric.Selection{selected(d, "4F", 15, "a"), selected(d, "4G", 30, "b"), selected(d, "4H", 45, "c")}, 45}, {[]cnvrubric.Selection{selected(d, "4L", 45, "a"), selected(d, "4M", 30, "b")}, 45}}
			for _, c := range cases {
				out := cnvrubric.Evaluate(engine.id, d, c.e, true)
				if out.TotalScoreHundredths != c.want {
					t.Fatalf("cap expected %d got %+v", c.want, out)
				}
			}
			duplicate := []cnvrubric.Selection{selected(d, "4A", 45, "same"), selected(d, "4B", 45, "same")}
			if cnvrubric.Evaluate(engine.id, d, duplicate, true).Status != "conflicting" {
				t.Fatal("same proband double counted")
			}
			invalid := selected(d, "4A", 46, "a")
			if cnvrubric.Evaluate(engine.id, d, []cnvrubric.Selection{invalid}, true).Classification != "insufficient_data" {
				t.Fatal("out of official range accepted")
			}
			conflict := []cnvrubric.Selection{selected(d, "1A", 0, "a"), selected(d, "1B", -60, "b")}
			if cnvrubric.Evaluate(engine.id, d, conflict, true).Status != "conflicting" {
				t.Fatal("mutually exclusive content accepted")
			}
		})
	}
}

func TestDuplicateIdentityWithinOneSelectionIsRejected(t *testing.T) {
	for _, item := range []struct {
		id    string
		rules []byte
	}{{"cnv_loss_2020", l.Rules}, {"cnv_gain_2020", g.Rules}} {
		defs := cnvrubric.Parse(item.rules)
		selection := selected(defs, "4A", 45, "same")
		selection.CaseIDs = []string{"same", "same"}
		result := cnvrubric.Evaluate(item.id, defs, []cnvrubric.Selection{selection}, true)
		if result.Status != "conflicting" || result.TotalScoreHundredths != 0 {
			t.Fatal("duplicate case contributed points", item.id, result)
		}
	}
}
func TestIndependentGeneCountThresholds(t *testing.T) {
	for _, c := range []struct {
		n    int
		gain bool
		code string
	}{{24, false, "3A"}, {25, false, "3B"}, {34, false, "3B"}, {35, false, "3C"}, {34, true, "3A"}, {35, true, "3B"}, {49, true, "3B"}, {50, true, "3C"}} {
		rules := l.Rules
		if c.gain {
			rules = g.Rules
		}
		d := cnvrubric.Parse(rules)
		score := int64(0)
		if c.code == "3B" {
			score = 45
		}
		if c.code == "3C" {
			score = 90
		}
		s := selected(d, c.code, score, "case")
		out := cnvrubric.WithAnnotation(cnv.AnnotatedCNV{GeneCount: c.n}, []cnvrubric.Selection{s}, c.gain)
		for _, v := range out[0].Facts {
			if !v {
				t.Fatalf("correct count rejected %+v", c)
			}
		}
		wrong := selected(d, "3C", 90, "case")
		if c.code != "3C" && cnvrubric.WithAnnotation(cnv.AnnotatedCNV{GeneCount: c.n}, []cnvrubric.Selection{wrong}, c.gain)[0].Facts["gene_count_upper"] {
			t.Fatal("incorrect count accepted")
		}
	}
}
