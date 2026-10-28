package assessment

import (
	"encoding/json"
	"reflect"
	"testing"
)

func factPackFixture(t *testing.T) Request {
	t.Helper()
	var r Request
	if err := json.Unmarshal([]byte(`{"engine_id":"cnv_loss_2020","cnv":{"assembly":"GRCh38","chrom":"1","start":1,"end":100,"type":"loss","coordinate_system":"1-based-inclusive"},"evidence_package":{"ai_context":{"variant_annotations":{"copyNumber":0,"sampleGenotypes":{"father":{"genotype":"./.","alleleFrequency":0}}},"sample":{"analysis_mode":"trio","age":0,"hpo_terms":[{"id":"HP:0001250"}]}}}}`), &r); err != nil {
		t.Fatal(err)
	}
	return r
}
func factByField(pack *AIFactPack, field string) AIImportedFact {
	for _, fact := range pack.Facts {
		if fact.Field == field {
			return fact
		}
	}
	return AIImportedFact{}
}

func TestRebuiltFactsKeepZeroAndRejectConfirmationClaims(t *testing.T) {
	r := factPackFixture(t)
	canonical := BuildImportedFactPack(r)
	if factByField(canonical, "annotation.copyNumber").Value != float64(0) || factByField(canonical, "cnv.copy_number").Value != float64(0) || factByField(canonical, "family.father.genotype").Status != "unknown" || factByField(canonical, "sample.age").Value != float64(0) {
		t.Fatal("zero/missing lost")
	}
	if canonical.InputStatus != "absent" || !canonical.ReportedIsNotVerified {
		t.Fatal("invalid import trust state")
	}
	r.Package.AIContext.FactPack = canonical
	if BuildImportedFactPack(r).InputStatus != "matched" {
		t.Fatal("canonical input rejected")
	}
	original := canonical.Facts[0]
	canonical.Facts[0].Value = "forged"
	canonical.Facts[0].Status = "verified"
	rebuilt := BuildImportedFactPack(r)
	if rebuilt.InputStatus != "mismatched" || !reflect.DeepEqual(rebuilt.Facts[0], original) {
		t.Fatal("caller verification accepted")
	}
	if canonical.Facts[0].Status != "verified" {
		t.Fatal("request mutated")
	}
	canonical.Version = "future"
	if BuildImportedFactPack(r).InputStatus != "unsupported_version" {
		t.Fatal("unknown version trusted")
	}
}

func TestRebuiltFactObjectConflictsAndEngineIsolation(t *testing.T) {
	if ValidCode("unknown-engine", "2A") {
		t.Fatal("unknown engine inherited CNV rules")
	}
	r := sequenceFixture()
	r.Package.AIContext = &AIContext{VariantAnnotations: map[string]any{"gene": "OTHER", "transcript": "OTHER", "copyNumber": float64(0)}}
	r.Variant.Gene = "FIXED"
	r.Variant.Transcript = "TRANSCRIPT"
	for _, engine := range []string{"acmg_amp_2015", "svc_v4"} {
		r.EngineID = engine
		p := BuildImportedFactPack(r)
		if factByField(p, "variant.gene").Status != "conflicting" || factByField(p, "annotation.copyNumber").Status != "not_applicable" {
			t.Fatal("conflict or engine boundary lost")
		}
	}
	cnv := factPackFixture(t)
	cnv.EngineID = "cnv_gain_2020"
	cnv.CNV.Type = "gain"
	if factByField(BuildImportedFactPack(cnv), "dosage.hi").Status != "not_applicable" {
		t.Fatal("gain borrowed HI")
	}
	cnv.EngineID = "cnv_loss_2020"
	cnv.CNV.Type = "loss"
	if factByField(BuildImportedFactPack(cnv), "dosage.ts").Status != "not_applicable" {
		t.Fatal("loss borrowed TS")
	}
}
