package api

import (
	"encoding/json"
	"github.com/pzweuj/sepilens/internal/assessment"
	"github.com/pzweuj/sepilens/internal/evidence"
	"strings"
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
	if len(facts) != 2 || facts[1]["field"] != "criterion.PP3.independent" {
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

func TestStructuredInputProvenanceUsesOpaqueAliasesAtEveryDepth(t *testing.T) {
	id := "PRIVATE_PATIENT_RECORD_ID"
	r := assessment.Request{EngineID: "acmg_amp_2015", Package: assessment.Package{Mechanism: &assessment.Mechanism{LOFEstablished: true, SourceIDs: []string{id}}}}
	source := evidence.Source{ID: id, DatasetVersion: "fixed-v1"}
	pack := modelFactPack(r, assessment.Result{}, []evidence.Source{source})
	encoded := opaqueCitations(pack, func(string) string { return "src_opaque" })
	raw, _ := json.Marshal(encoded)
	if strings.Contains(string(raw), id) || !strings.Contains(string(raw), `"source_ids":["src_opaque"]`) || !strings.Contains(string(raw), `"id":"src_opaque"`) {
		t.Fatal("raw source identity escaped through structured provenance")
	}
}
