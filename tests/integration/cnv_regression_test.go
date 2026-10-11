package integration_test

import (
	"encoding/json"
	g "github.com/pzweuj/sepilens/internal/rules/cnvgain2020"
	l "github.com/pzweuj/sepilens/internal/rules/cnvloss2020"
	"github.com/pzweuj/sepilens/internal/variant/cnv"
	"os"
	"testing"
)

func TestLegacyCNVFixturesDoNotImplyCoverage(t *testing.T) {
	for _, kind := range []string{"cnv_loss", "cnv_gain"} {
		b, e := os.ReadFile("../fixtures/" + kind + "/golden_cases.json")
		if e != nil {
			t.Fatal(e)
		}
		var cases []struct {
			ID  string       `json:"id"`
			CNV cnv.CNVInput `json:"cnv"`
		}
		if e = json.Unmarshal(b, &cases); e != nil {
			t.Fatal(e)
		}
		for _, c := range cases {
			t.Run(c.ID, func(t *testing.T) {
				ann := cnv.NewAnnotator().Annotate(c.CNV)
				r := l.NewEngine().Evaluate(l.LossEvaluationInput{CNV: ann})
				if kind == "cnv_gain" {
					r = g.NewEngine().Evaluate(g.GainEvaluationInput{CNV: ann})
				}
				if r.Classification != "insufficient_data" || len(r.Clauses) != 0 {
					t.Fatalf("unverified legacy data scored: %+v", r)
				}
			})
		}
	}
}
