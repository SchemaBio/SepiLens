package assessment

import (
	"testing"
	"time"

	"github.com/pzweuj/sepilens/internal/annotation"
	"github.com/pzweuj/sepilens/internal/config"
	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
	"github.com/pzweuj/sepilens/internal/variant/cnv"
)

func sequenceFixture() Request {
	score := 0.99
	r := Request{SchemaVersion: SchemaVersion, EngineID: "acmg_amp_2015", Mode: "local", Variant: &acmg2015.SNVVariantInput{Assembly: "GRCh38", Chrom: "1", Pos: 100, Ref: "A", Alt: "G", Gene: "G", Transcript: "NM_1.1", Consequence: "missense_variant"}, Context: acmg2015.ClinicalContextInput{DiseaseID: "MONDO:1", Inheritance: "AD", VariantOrigin: "germline_confirmed", SampleType: "blood"}}
	r.Package.Annotations = &annotation.Set{Source: annotation.SourceSnapshot, AlphaMissense: &annotation.AlphaMissense{Record: annotation.Record{Version: "test", RetrievedAt: time.Now().UTC().Format(time.RFC3339), Status: annotation.StatusFound}, Score: &score, Class: "likely_pathogenic", Transcript: "NM_1.1"}}
	r.Package.Sources = []Source{{ID: "am", Kind: "alphamissense", Dataset: "test", Version: "test", RetrievedAt: time.Now().UTC().Format(time.RFC3339), RecordID: "record", LicenseConfirmed: true, Object: ObjectFor(r)}}
	return r
}
func testPolicy() map[string]config.SourcePolicy {
	return map[string]config.SourcePolicy{"test": {Versions: []string{"test"}, Actors: []string{"yijian"}, Assemblies: []string{"GRCh38"}, Clinical: true}}
}
func TestExternalSourcePolicyCannotBeClaimed(t *testing.T) {
	for _, scenario := range []string{"accepted", "unknown_actor", "old", "licence", "version", "policy_missing"} {
		t.Run(scenario, func(t *testing.T) {
			r := sequenceFixture()
			actor := "yijian"
			policies := testPolicy()
			switch scenario {
			case "unknown_actor":
				actor = "attacker"
			case "old":
				r.Package.Sources[0].RetrievedAt = time.Now().Add(-100 * 24 * time.Hour).UTC().Format(time.RFC3339)
			case "licence":
				r.Package.Sources[0].LicenseConfirmed = false
			case "version":
				r.Package.Sources[0].Version = "other"
			case "policy_missing":
				policies = nil
			}
			n, err := Normalize(r)
			if err != nil {
				t.Fatal(err)
			}
			sources, _ := Sources(n, actor, policies, time.Now().UTC())
			out := Evaluate(n, sources)
			met := false
			for _, c := range out.Criteria {
				met = met || (c.Code == "PP3" && c.Status == "met")
			}
			if met != (scenario == "accepted") {
				t.Fatalf("trust decision wrong: %+v", out)
			}
			if r.Package.Annotations.Source != annotation.SourceSnapshot {
				t.Fatal("caller mutated")
			}
		})
	}
}
func TestCNVDosageBothDirectionsAndCoverage(t *testing.T) {
	for _, gain := range []bool{false, true} {
		for _, complete := range []bool{false, true} {
			r := sequenceFixture()
			r.Variant = nil
			r.Package.Annotations = nil
			r.EngineID = "cnv_loss_2020"
			typ := "loss"
			if gain {
				typ = "gain"
				r.EngineID = "cnv_gain_2020"
			}
			r.CNV = &Region{CNVInput: cnv.CNVInput{Assembly: "GRCh38", Chrom: "1", Start: 1, End: 100, Type: typ}, CoordinateSystem: "1-based-inclusive"}
			r.Package.Sources[0].Kind = "dosage"
			r.Package.Sources[0].Object = ObjectFor(r)
			r.Package.Sources[0].Complete = complete
			r.Package.Dosage = &Dosage{Coverage: ObjectFor(r), Complete: complete, SourceIDs: []string{"am"}, Regions: []cnv.DosageRegion{{Name: "G", Assembly: "GRCh38", Chrom: "1", Start: 10, End: 90, HIScore: 3, TSScore: 3, Version: "test"}}}
			n, e := Normalize(r)
			if e != nil {
				t.Fatal(e)
			}
			sources, _ := Sources(n, "yijian", testPolicy(), time.Now().UTC())
			out := Evaluate(n, sources)
			if complete && out.Classification != "Pathogenic" {
				t.Fatal(out)
			}
			if !complete && out.Status != "insufficient_data" {
				t.Fatal("incomplete coverage scored", out)
			}
		}
	}
}
func TestObjectHashAndUnknownFields(t *testing.T) {
	r := sequenceFixture()
	r.Package.ContentHash = HashPackage(r.Package)
	if _, e := Normalize(r); e != nil {
		t.Fatal(e)
	}
	r.Package.Sources[0].RecordID = "tamper"
	if _, e := Normalize(r); e == nil {
		t.Fatal("hash tamper accepted")
	}
	r.Package.ContentHash = ""
	r.Package.Sources[0].Object.Pos++
	if _, e := Normalize(r); e == nil {
		t.Fatal("object mismatch accepted")
	}
}
func TestPVS1RequiresMechanismAndTranscriptEvidence(t *testing.T) {
	r := sequenceFixture()
	r.Variant.Consequence = "stop_gained"
	r.Package.Annotations.AlphaMissense = nil
	r.Package.Annotations.GenCC = &annotation.GenCC{Record: annotation.Record{Version: "test", RetrievedAt: time.Now().UTC().Format(time.RFC3339)}, Gene: "G", DiseaseID: "MONDO:1", MOI: "AD", Classification: "Definitive"}
	r.Package.Sources[0].Kind = "gencc"
	r.Package.Sources[0].Object = ObjectFor(r)
	sources, _ := Sources(r, "yijian", testPolicy(), time.Now().UTC())
	res := Evaluate(r, sources)
	for _, c := range res.Criteria {
		if c.Code == "PVS1" && c.Status == "met" {
			t.Fatal("inferred mechanism")
		}
	}
}
