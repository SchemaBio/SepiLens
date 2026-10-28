package assessment

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/rules/svcv4"
	"github.com/pzweuj/sepilens/internal/variant/cnv"
)

func structuredFact(pack AIStructuredInputs, field string) AIStructuredFact {
	for _, fact := range pack.Facts {
		if fact.Field == field {
			return fact
		}
	}
	return AIStructuredFact{}
}
func structuredSource() evidence.Source {
	return evidence.Source{ID: "source", DatasetVersion: "fixed-v1", IdentifierValid: true, RecordExists: true, ObjectMatched: true, MaterialVerified: true, ValidationStatus: evidence.ValidationVerified}
}
func TestStructuredDosageKeepsDirectionZeroAndFalseWithoutInventingProof(t *testing.T) {
	for _, engine := range []string{"cnv_loss_2020", "cnv_gain_2020"} {
		r := factPackFixture(t)
		r.EngineID = engine
		if engine == "cnv_gain_2020" {
			r.CNV.Type = "gain"
		}
		zero, no := 0, false
		r.Package.Sources = []Source{{ID: "source", Version: "fixed-v1", Complete: true}}
		r.Package.Dosage = &Dosage{SourceIDs: []string{"source"}, Complete: true, Coverage: ObjectFor(r), ProteinCodingGenes: &zero, GeneCountSourceIDs: []string{"source"}, FunctionalElements: &no, FunctionalSourceIDs: []string{"source"}, Regions: []cnv.DosageRegion{{Name: "G", Assembly: "GRCh38", Chrom: "1", Start: 10, End: 90, HIScore: 0, TSScore: 3, Version: "fixed-v1"}}}
		before, _ := json.Marshal(r)
		unverified := BuildStructuredInputFacts(r, nil)
		if structuredFact(unverified, "input.dosage.complete_overlap").Status != "unknown" || structuredFact(unverified, "input.dosage.protein_coding_genes").Confirmation != "unconfirmed" {
			t.Fatal("request established its own dosage truth")
		}
		pack := BuildStructuredInputFacts(r, []evidence.Source{structuredSource()})
		count := structuredFact(pack, "input.dosage.protein_coding_genes")
		functional := structuredFact(pack, "input.dosage.functional_elements")
		if count.Value != float64(0) || functional.Value != false || count.Confirmation != "source_validated" || count.Status != "reported" || count.SourceVersions[0]["version"] != "fixed-v1" {
			t.Fatal("explicit zero/false or provenance lost")
		}
		wanted, forbidden := "hi_score", "ts_score"
		overlap := false
		if engine == "cnv_gain_2020" {
			wanted, forbidden = "ts_score", "hi_score"
			overlap = true
		}
		if structuredFact(pack, "input.dosage.regions.0."+wanted).Status != "reported" || structuredFact(pack, "input.dosage.regions.0."+forbidden).Field != "" || structuredFact(pack, "input.dosage.complete_overlap").Value != overlap {
			t.Fatal("engine direction/annotator parity lost")
		}
		after, _ := json.Marshal(r)
		if string(before) != string(after) {
			t.Fatal("input mutated")
		}
		source := structuredSource()
		source.ObjectMatched = false
		if structuredFact(BuildStructuredInputFacts(r, []evidence.Source{source}), "input.dosage.protein_coding_genes").Confirmation != "unconfirmed" {
			t.Fatal("object mismatch confirmed")
		}
		source = structuredSource()
		source.ValidationStatus = evidence.ValidationConflict
		if structuredFact(BuildStructuredInputFacts(r, []evidence.Source{source}), "input.dosage.protein_coding_genes").Status != "conflicting" {
			t.Fatal("conflict hidden")
		}
	}
}
func TestStructuredSVCMethodsDoNotInferValidityOrExposeCaseIdentity(t *testing.T) {
	r := Request{EngineID: "svc_v4", Package: Package{DraftSourceIDs: []string{"source"}, Sources: []Source{{ID: "source", Clinical: true, Version: "fixed-v1"}}, Draft: &svcv4.DraftAssessment{MOI: "AR", Population: &svcv4.Population{}, Impact: &svcv4.Impact{Workflow: "NUCLEOTIDE", Outcome: "NOT_A_VALIDATED_METHOD"}, Cases: []svcv4.Case{{ID: "PRIVATE_PATIENT", FamilyID: "PRIVATE_FAMILY", Parentage: "UNKNOWN", Zygosity: "HET"}}, CaseControl: &svcv4.CaseControl{}}}}
	pack := BuildStructuredInputFacts(r, []evidence.Source{structuredSource()})
	if structuredFact(pack, "input.svc.gene_disease_validity").Status != "unknown" || structuredFact(pack, "input.svc.cases.0.confirmed_parental_relationship").Value != "UNKNOWN" || structuredFact(pack, "input.svc.case_control.controls_matched").Status != "unknown" {
		t.Fatal("missing prerequisites inferred")
	}
	if structuredFact(pack, "input.svc.variant_impact.workflow").Status != "reported" {
		t.Fatal("reported method lost")
	}
	raw, _ := json.Marshal(pack)
	if strings.Contains(string(raw), "PRIVATE_PATIENT") || strings.Contains(string(raw), "PRIVATE_FAMILY") {
		t.Fatal("case identities exposed")
	}
	for _, fact := range pack.Facts {
		if fact.Status == "supported" {
			t.Fatal("method inputs approved a rule")
		}
	}
	r.Package.Sources[0].Clinical = false
	if structuredFact(BuildStructuredInputFacts(r, []evidence.Source{structuredSource()}), "input.svc.moi").Confirmation != "unconfirmed" {
		t.Fatal("nonclinical source confirmed a method input")
	}
}
func TestStructuredInputsAreBoundedAndDoNotHideOmissions(t *testing.T) {
	r := Request{EngineID: "svc_v4", Package: Package{Draft: &svcv4.DraftAssessment{MOI: "AR", Cases: make([]svcv4.Case, 200)}}}
	pack := BuildStructuredInputFacts(r, nil)
	if len(pack.Facts) != 256 || pack.OmittedCount <= 32 || len(pack.OmittedFields) != 32 {
		t.Fatal("unbounded context or silent omissions")
	}
	if len(BuildStructuredInputFacts(Request{EngineID: "unknown"}, nil).Facts) != 0 {
		t.Fatal("unknown engine inherited facts")
	}
	r = sequenceFixture()
	r.Package.Mechanism = &Mechanism{Transcript: "NM_FIXED"}
	pack = BuildStructuredInputFacts(r, nil)
	if structuredFact(pack, "input.mechanism.lof_established").Status != "unknown" || structuredFact(pack, "input.mechanism.transcript").Value != "NM_FIXED" {
		t.Fatal("decoder defaults became negative evidence")
	}
}
