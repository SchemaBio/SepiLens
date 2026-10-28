package api

import (
	"github.com/pzweuj/sepilens/internal/assessment"
	"github.com/pzweuj/sepilens/internal/evidence"
	"testing"
)

func TestEvidenceQualityDoesNotTrustModelFacts(t *testing.T) {
	for _, code := range []string{"PVS1", "POP", "2A"} {
		c := assessment.Criterion{Code: code, Status: "met", SourceIDs: []string{"s"}, Facts: map[string]bool{"independent": true}}
		p := assessmentProposal{Criteria: []assessmentChange{{Criterion: c}}}
		source := evidence.Source{ID: "s", IdentifierValid: true, RecordExists: true, ObjectMatched: true, MaterialVerified: true, ValidationStatus: evidence.ValidationVerified}
		engine := map[string]string{"PVS1": "acmg_amp_2015", "POP": "svc_v4", "2A": "cnv_loss_2020"}[code]
		base := assessment.Result{EngineID: engine, Criteria: []assessment.Criterion{c}}
		if evidenceQuality(base, p, []evidence.Source{source})["supported_count"] != 1 {
			t.Fatal("supported baseline lost")
		}
		if evidenceQuality(assessment.Result{}, p, []evidence.Source{source})["supported_count"] != 0 {
			t.Fatal("model established applicability")
		}
		source.ObjectMatched = false
		if evidenceQuality(base, p, []evidence.Source{source})["supported_count"] != 0 {
			t.Fatal("unmatched object supported")
		}
		source.ObjectMatched = true
		source.MaterialVerified = false
		if evidenceQuality(base, p, []evidence.Source{source})["supported_count"] != 0 {
			t.Fatal("record existence substituted for material")
		}
	}
}

func TestModelFactPackOnlyAddsIndependentlyEstablishedFacts(t *testing.T) {
	r := assessment.Request{EngineID: "acmg_amp_2015"}
	source := evidence.Source{ID: "private-source-record", IdentifierValid: true, RecordExists: true, ObjectMatched: true, MaterialVerified: true, ValidationStatus: evidence.ValidationVerified, DatasetVersion: "fixed-fixture-v1"}
	base := assessment.Result{EngineID: r.EngineID, Criteria: []assessment.Criterion{{Code: "PP3", Status: "met", Facts: map[string]bool{"independent": true}, SourceIDs: []string{source.ID}}}}
	pack := modelFactPack(r, base, []evidence.Source{source})
	facts := pack["established_facts"].([]map[string]any)
	if len(facts) != 1 || facts[0]["field"] != "criterion.PP3.independent" {
		t.Fatal("established fact missing")
	}
	encoded := opaqueCitations(pack, func(string) string { return "src_1" }).(map[string]any)
	if encoded["established_facts"].([]any)[0].(map[string]any)["source_ids"].([]string)[0] != "src_1" {
		t.Fatal("source identity not aliased")
	}
	source.ObjectMatched = false
	if len(modelFactPack(r, base, []evidence.Source{source})["established_facts"].([]map[string]any)) != 0 {
		t.Fatal("object mismatch accepted")
	}
	r.EngineID = "svc_v4"
	source.ObjectMatched = true
	if len(modelFactPack(r, base, []evidence.Source{source})["established_facts"].([]map[string]any)) != 0 {
		t.Fatal("ACMG fact crossed engine boundary")
	}
}
