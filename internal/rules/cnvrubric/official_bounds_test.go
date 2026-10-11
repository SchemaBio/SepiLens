package cnvrubric_test

import (
	"fmt"
	"testing"

	g "github.com/pzweuj/sepilens/internal/rules/cnvgain2020"
	l "github.com/pzweuj/sepilens/internal/rules/cnvloss2020"
	"github.com/pzweuj/sepilens/internal/rules/cnvrubric"
)

// Independent numeric transcription of the published ClinGen calculators,
// checked 2026-10-11. This is an engineering rubric contract, not expert-approved
// patient golden cases. Values are hundredths: min, max, aggregate min/max.
// https://cnvcalc.clinicalgenome.org/cnvcalc/cnv-loss
// https://cnvcalc.clinicalgenome.org/cnvcalc/cnv-gain
func TestEveryPublishedClauseBoundsAndMaterialGate(t *testing.T) {
	common := map[string][4]int64{
		"1A": {0, 0, 0, 0}, "1B": {-60, -60, -60, -60}, "2A": {100, 100, 100, 100}, "2B": {0, 0, 0, 0},
		"3A": {0, 0, 0, 0}, "3B": {45, 45, 45, 45}, "3C": {90, 90, 90, 90},
		"4A": {15, 45, 0, 90}, "4B": {0, 45, 0, 90}, "4C": {0, 30, 0, 90}, "4D": {-30, 0, -30, 0}, "4E": {0, 15, 0, 30},
		"4F": {15, 15, 0, 45}, "4G": {30, 30, 0, 45}, "4H": {45, 45, 0, 45},
		"4I": {-45, 0, -90, 0}, "4J": {-30, 0, -90, 0}, "4K": {-15, 0, -30, 0},
		"4L": {0, 45, 0, 45}, "4M": {0, 30, 0, 45}, "4N": {-90, 0, -90, 0}, "4O": {-100, 0, -100, 0},
		"5A": {-30, 45, -30, 45}, "5B": {-45, 0, -45, 0}, "5C": {-30, 0, -30, 0}, "5D": {0, 45, 0, 45},
		"5E": {-45, 0, -45, 0}, "5F": {0, 0, 0, 0}, "5G": {0, 15, 0, 15}, "5H": {0, 30, 0, 30},
	}
	loss := map[string][4]int64{
		"2C-1": {45, 100, 45, 100}, "2C-2": {0, 45, 0, 45}, "2D-1": {0, 0, 0, 0}, "2D-2": {45, 90, 45, 90}, "2D-3": {0, 45, 0, 45}, "2D-4": {45, 100, 45, 100},
		"2E-PVS1": {45, 90, 45, 90}, "2E-Strong": {30, 90, 30, 90}, "2E-Moderate": {15, 45, 15, 45}, "2E-Supporting": {0, 30, 0, 30}, "2E-NA": {0, 0, 0, 0},
		"2F": {-100, -100, -100, -100}, "2G": {0, 0, 0, 0}, "2H": {15, 15, 15, 15},
	}
	gain := map[string][4]int64{
		"2C": {-100, -100, -100, -100}, "2D": {-100, -100, -100, -100}, "2E": {0, 0, 0, 0}, "2F": {-100, 0, -100, 0}, "2G": {0, 0, 0, 0}, "2H": {0, 0, 0, 0},
		"2I-PVS1": {45, 90, 45, 90}, "2I-Strong": {30, 90, 30, 90}, "2I-NA": {0, 0, 0, 0}, "2J": {0, 0, 0, 0}, "2K": {45, 45, 45, 45}, "2L": {0, 0, 0, 0},
	}
	for _, target := range []struct {
		id       string
		rules    []byte
		specific map[string][4]int64
	}{{"cnv_loss_2020", l.Rules, loss}, {"cnv_gain_2020", g.Rules, gain}} {
		defs := cnvrubric.Parse(target.rules)
		if len(defs) != len(common)+len(target.specific) {
			t.Fatalf("%s missing published clause", target.id)
		}
		for _, d := range defs {
			want, ok := common[d.Code]
			if !ok {
				want, ok = target.specific[d.Code]
			}
			if !ok {
				t.Fatalf("undocumented clause %s", d.Code)
			}
			t.Run(target.id+"/"+d.Code, func(t *testing.T) {
				if got := [4]int64{d.Min, d.Max, d.CapMin, d.CapMax}; got != want {
					t.Fatalf("published bounds %v, got %v", want, got)
				}
				for _, score := range []int64{want[0], want[1]} {
					s := selected(defs, d.Code, score, "case-or-study-1")
					r := cnvrubric.Evaluate(target.id, defs, []cnvrubric.Selection{s}, true)
					if len(r.Clauses) != 1 || r.TotalScoreHundredths != score {
						t.Fatalf("valid published boundary rejected: %+v", r)
					}
					s.Verified = false
					if r = cnvrubric.Evaluate(target.id, defs, []cnvrubric.Selection{s}, true); len(r.Clauses) != 0 || r.Status != "insufficient_data" {
						t.Fatal("unverified material counted")
					}
				}
				for _, invalid := range []int64{want[0] - 1, want[1] + 1} {
					if r := cnvrubric.Evaluate(target.id, defs, []cnvrubric.Selection{selected(defs, d.Code, invalid, "case")}, true); len(r.Clauses) != 0 {
						t.Fatal("out-of-range score counted")
					}
				}
				for _, fact := range d.Facts {
					s := selected(defs, d.Code, want[1], "case")
					delete(s.Facts, fact)
					if r := cnvrubric.Evaluate(target.id, defs, []cnvrubric.Selection{s}, true); len(r.Clauses) != 0 {
						t.Fatalf("missing %s counted", fact)
					}
				}
				var repeated []cnvrubric.Selection
				for i := 0; i < 10; i++ {
					score := want[1]
					if score == 0 {
						score = want[0]
					}
					repeated = append(repeated, selected(defs, d.Code, score, fmt.Sprint("case-", i)))
				}
				r := cnvrubric.Evaluate(target.id, defs, repeated, true)
				if r.TotalScoreHundredths < want[2] || r.TotalScoreHundredths > want[3] {
					t.Fatalf("aggregate cap exceeded: %+v", r)
				}
			})
		}
	}
}
