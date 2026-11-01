package retrieval

import (
	"strings"
	"testing"
)

func TestSequenceQueryUsesPinnedDNAAndProteinAlternatives(t *testing.T) {
	q := Query{Gene: "MYH3", HGVS: "NM_002470.4:c.2015G>A", ProteinHGVS: "NP_002461.2:p.Arg672His"}
	term := sequenceLiteratureTerm(q)
	for _, value := range []string{"MYH3", `"c.2015G>A"`, `"Arg672His"`, `"R672H"`} {
		if !strings.Contains(term, value) {
			t.Fatal("missing exact variant alternative", term)
		}
	}
	if !sequenceObjectMatched(q, "MYH3 R672H affects contraction") || sequenceObjectMatched(q, "MYH3 Arg672Cys study") || sequenceObjectMatched(q, "MYH13 R672H") {
		t.Fatal("variant or gene association was lost")
	}
}
