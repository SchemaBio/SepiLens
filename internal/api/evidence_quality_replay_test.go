package api

import (
	"fmt"
	"github.com/pzweuj/sepilens/internal/assessment"
	"github.com/pzweuj/sepilens/internal/evidence"
	"testing"
)

// Engineering replay matrix, not independently reviewed clinical labels.
// Each engine covers six failure modes at five distinct clause/source inputs.
func TestEvidenceQualityReplayMatrix(t *testing.T) {
	engines := map[string][]string{"acmg_amp_2015": {"PVS1", "PS1", "PM2", "PP3", "BP4"}, "cnv_loss_2020": {"1A", "2A", "3A", "4A", "5A"}, "cnv_gain_2020": {"1A", "2A", "3A", "4A", "5A"}, "svc_v4": {"POP", "CLN", "LOC", "NUL", "CDS"}}
	for engine, codes := range engines {
		for variant, code := range codes {
			for _, scenario := range []string{"supported", "object_mismatch", "material_missing", "missing_facts", "conflict", "duplicate"} {
				t.Run(fmt.Sprintf("%s/%s/%d", engine, scenario, variant), func(t *testing.T) {
					source := evidence.Source{ID: fmt.Sprintf("s%d", variant), IdentifierValid: true, RecordExists: true, ObjectMatched: true, MaterialVerified: true, ValidationStatus: evidence.ValidationVerified}
					c := assessment.Criterion{Code: code, Status: "met", SourceIDs: []string{source.ID}, Facts: map[string]bool{"independent": true}}
					b := c
					b.Facts = map[string]bool{"independent": true}
					p := assessmentProposal{Criteria: []assessmentChange{{Criterion: c}}}
					switch scenario {
					case "object_mismatch":
						source.ObjectMatched = false
					case "material_missing":
						source.MaterialVerified = false
					case "missing_facts":
						b.Facts = nil
					case "conflict":
						p.Criteria[0].Status = "conflicting"
					case "duplicate":
						p.Criteria = append(p.Criteria, p.Criteria[0])
					}
					q := evidenceQuality(assessment.Result{EngineID: engine, Criteria: []assessment.Criterion{b}}, p, []evidence.Source{source})
					expected := 0
					if scenario == "supported" {
						expected = 1
					}
					if q["supported_count"] != expected {
						t.Fatalf("support=%v expected=%d", q["supported_count"], expected)
					}
				})
			}
		}
	}
}
