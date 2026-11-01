package assessment

import "testing"

func TestPublicMYH3MissenseSurvivesImmutableNormalization(t *testing.T) {
	r := sequenceFixture()
	r.Variant.Assembly = "GRCh37"
	r.Variant.Chrom = "17"
	r.Variant.Pos = 10544634
	r.Variant.Ref = "C"
	r.Variant.Alt = "T"
	r.Variant.Gene = "MYH3"
	r.Variant.Transcript = "NM_002470.4"
	r.Variant.HGVSc = "c.2015G>A"
	r.Variant.HGVSp = "p.Arg672His"
	r.Variant.Consequence = "missense variant"
	r.Package.Annotations = nil
	r.Package.Sources = nil
	n, err := Normalize(r)
	if err != nil {
		t.Fatal(err)
	}
	if n.Variant.Consequence != "missense_variant" || n.Variant.Pos != 10544634 || n.Variant.HGVSc != "c.2015G>A" || r.Variant.Consequence != "missense variant" {
		t.Fatal("normalization lost identity or mutated raw context")
	}
}
