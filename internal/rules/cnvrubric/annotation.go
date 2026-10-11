package cnvrubric

import (
	"github.com/pzweuj/sepilens/internal/variant/cnv"
)

func WithAnnotation(a cnv.AnnotatedCNV, selections []Selection, gain bool) []Selection {
	out := append([]Selection(nil), selections...)
	lower, middle := 25, 35
	if gain {
		lower, middle = 35, 50
	}
	for i := range out {
		s := &out[i]
		s.Facts = cloneFacts(s.Facts)
		check := func(f string, observed bool) {
			if _, claimed := s.Facts[f]; claimed {
				s.Facts[f] = s.Facts[f] && observed
			}
		}
		check("functional_elements", a.ContainsFunctionalElements)
		check("no_functional_elements", !a.ContainsFunctionalElements)
		check("complete_hi_overlap", a.CompleteHIOverlap)
		check("complete_ts_overlap", a.CompleteTSOverlap)
		check("contained_benign_loss", a.CompletelyInBenign)
		check("gene_count_lower", a.GeneCount < lower)
		check("gene_count_middle", a.GeneCount >= lower && a.GeneCount < middle)
		check("gene_count_upper", a.GeneCount >= middle)
		for _, f := range []string{"intragenic_pvs1", "intragenic_pvs1_strong", "intragenic_pvs1_moderate", "intragenic_pvs1_supporting", "intragenic_not_applicable"} {
			check(f, a.IsIntragenic && a.Input.BreakpointPrecision == "exact" && (!gain || a.Input.Orientation == "tandem"))
		}
		for _, f := range []string{"hi_five_prime_coding", "hi_five_prime_utr", "hi_three_prime_utr", "hi_last_exon_pathogenic", "hi_last_exon_no_pathogenic", "hi_three_prime_nmd", "hi_breakpoint_unknown_phenotype", "hi_breakpoint_specific_phenotype", "other_gene_breakpoint"} {
			check(f, a.Input.BreakpointPrecision == "exact")
		}
		if a.Input.Mosaicism == "present" {
			// Mosaic events require a separately validated applicability model.
			for f := range s.Facts {
				s.Facts[f] = false
			}
		}
	}
	return out
}
func cloneFacts(in map[string]bool) map[string]bool {
	out := map[string]bool{}
	for k, v := range in {
		out[k] = v
	}
	return out
}
