package api

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/pzweuj/sepilens/internal/assessment"
	"github.com/pzweuj/sepilens/internal/evidence"
)

func bindingFixture() (assessmentChange, map[string]bindingFact, []evidence.Source, map[string]any) {
	source := evidence.Source{ID: "record-one", IdentifierValid: true, RecordExists: true, ObjectMatched: true, MaterialVerified: true, ValidationStatus: evidence.ValidationVerified, DatasetVersion: "synthetic-v1", Snippet: "Synthetic source material about a fixed evaluation object."}
	c := assessmentChange{Criterion: assessment.Criterion{Code: "PP3", Status: "met", Strength: "Supporting", Facts: map[string]bool{"computational": true}, SourceIDs: []string{source.ID}}}
	pack := modelFactPack(assessment.Request{EngineID: "acmg_amp_2015"}, assessment.Result{EngineID: "acmg_amp_2015", Criteria: []assessment.Criterion{c.Criterion}}, []evidence.Source{source})
	return c, evidenceBindingCatalog(pack), []evidence.Source{source}, pack
}
func hasBindingIssue(issues []string, expected string) bool {
	for _, issue := range issues {
		if issue == expected {
			return true
		}
	}
	return false
}

func TestBindingsUseIndependentStructuredProofAndPreserveOrigin(t *testing.T) {
	c, catalog, sources, _ := bindingFixture()
	checked, issues := checkEvidenceBindings(c, catalog, sources)
	if len(issues) != 0 || len(checked) != 1 || checked[0].Status != "supported" || checked[0].Origin != "rule_engine" || checked[0].Path != "/rule_engine_result/criteria/PP3/facts/computational" {
		t.Fatal("baseline proof lost")
	}
	c.Bindings = []modelEvidenceBinding{{Fact: "computational", FactID: "criterion.PP3.computational", SourceID: "record-one"}}
	checked, issues = checkEvidenceBindings(c, catalog, sources)
	if len(issues) != 0 || len(checked) != 1 || checked[0].Origin != "model" || checked[0].Status != "supported" {
		t.Fatal("valid structured binding rejected")
	}
}

func TestExcerptTraceabilityNeverVerifiesClinicalEntailment(t *testing.T) {
	c, _, sources, _ := bindingFixture()
	c.Bindings = []modelEvidenceBinding{{Fact: "computational", SourceID: "record-one", Excerpt: "Synthetic source material about a fixed evaluation object."}}
	checked, issues := checkEvidenceBindings(c, map[string]bindingFact{}, sources)
	if len(issues) != 0 || len(checked) != 1 || checked[0].Status != "pending_review" || checked[0].Reason != "excerpt_requires_review" {
		t.Fatal("text matching promoted a clinical claim")
	}
}

func TestBindingsRejectWrongScopeInventedExcerptsAndUnavailableFacts(t *testing.T) {
	for _, scenario := range []string{"unknown_fact_id", "fact_scope_mismatch", "unknown_source", "source_not_cited", "excerpt_not_located", "referenced_fact_unavailable", "request_fact_cannot_claim_source", "source_conflicting_or_rejected", "ambiguous_binding"} {
		t.Run(scenario, func(t *testing.T) {
			c, catalog, sources, _ := bindingFixture()
			binding := modelEvidenceBinding{Fact: "computational", SourceID: "record-one", Excerpt: sources[0].Snippet}
			switch scenario {
			case "unknown_fact_id":
				binding.FactID = "PRIVATE_PATIENT_VALUE"
				binding.Excerpt = ""
			case "fact_scope_mismatch":
				catalog["criterion.PM2.other"] = bindingFact{Field: "criterion.PM2.other", Code: "PM2", Status: "supported"}
				binding.FactID = "criterion.PM2.other"
				binding.Excerpt = ""
			case "unknown_source":
				binding.SourceID = "SECRET_PROVIDER_CONTENT"
			case "source_not_cited":
				sources = append(sources, evidence.Source{ID: "record-two", Snippet: sources[0].Snippet})
				binding.SourceID = "record-two"
			case "excerpt_not_located":
				binding.Excerpt = "Invented source sentence that was never supplied."
			case "referenced_fact_unavailable":
				catalog["family.de_novo_confirmed"] = bindingFact{Field: "family.de_novo_confirmed", Status: "unknown"}
				binding.FactID = "family.de_novo_confirmed"
				binding.SourceID = ""
				binding.Excerpt = ""
			case "request_fact_cannot_claim_source":
				catalog["annotation.gnomadAF"] = bindingFact{Field: "annotation.gnomadAF", Status: "reported", Value: float64(0)}
				binding.FactID = "annotation.gnomadAF"
				binding.Excerpt = ""
			case "source_conflicting_or_rejected":
				sources[0].ValidationStatus = evidence.ValidationRejected
			case "ambiguous_binding":
				binding.FactID = "criterion.PP3.computational"
			}
			c.Bindings = []modelEvidenceBinding{binding}
			_, issues := checkEvidenceBindings(c, catalog, sources)
			if !hasBindingIssue(issues, scenario) {
				t.Fatal("bad binding not identified", scenario, issues)
			}
		})
	}
}

func TestReportedGenotypesCannotProveClinicalPrerequisites(t *testing.T) {
	c, _, sources, _ := bindingFixture()
	c.Bindings = []modelEvidenceBinding{{Fact: "computational", FactID: "family.proband.genotype"}}
	catalog := map[string]bindingFact{"family.proband.genotype": {Field: "family.proband.genotype", Value: "0/1", Status: "reported", Path: "/request/genotype"}}
	checked, issues := checkEvidenceBindings(c, catalog, sources)
	if checked[0].Status != "context_only" || !hasBindingIssue(issues, "missing_independent_or_excerpt_binding") {
		t.Fatal("imported genotype proved medical fact")
	}
}

func TestIndependentlyMatchedConflictHoldsPriorPaperWithoutTrustPoisoning(t *testing.T) {
	c, catalog, sources, _ := bindingFixture()
	sources[0].PMID = "123"
	conflict := sources[0]
	conflict.ID = "counter-record"
	conflict.ValidationStatus = evidence.ValidationConflict
	_, issues := checkEvidenceBindings(c, catalog, append(sources, conflict))
	if len(issues) == 0 {
		t.Fatal("independent conflict ignored")
	}
	conflict.MaterialVerified = false
	checked, issues := checkEvidenceBindings(c, catalog, append(sources, conflict))
	if len(issues) != 0 || len(checked) != 1 || checked[0].Status != "supported" {
		t.Fatal("unverified search result poisoned baseline")
	}
}

func TestBindingProjectionKeepsValidClausesAndPrivateDiagnosticsBounded(t *testing.T) {
	good, _, sources, pack := bindingFixture()
	bad := good
	bad.Code = "PM2"
	bad.Facts = map[string]bool{"frequency": true}
	bad.Bindings = []modelEvidenceBinding{{Fact: "frequency", FactID: "PRIVATE_PATIENT_VALUE"}}
	p := assessmentProposal{ClinicalSummary: "Synthetic summary", Classification: "Pathogenic", Criteria: []assessmentChange{bad, good}}
	projected, reports, warnings := projectEvidenceBindings(p, pack, sources)
	if projected.Criteria[0].Status != "insufficient_data" || projected.Criteria[0].Score != nil || len(projected.Criteria[0].Facts) != 0 || !reflect.DeepEqual(projected.Criteria[1], good) || len(reports) != 2 || len(warnings) != 1 || projected.Classification != "insufficient_data" {
		t.Fatal("selective projection failed")
	}
	wire := modelProposal{Criteria: []modelChange{{Code: "PM2", Status: "met", SourceIDs: []string{"src_1"}, Facts: []string{"frequency"}, Bindings: bad.Bindings}}}
	raw, _ := json.Marshal(wire)
	issues := evidenceBindingReview(raw, pack, sources, map[string]string{"src_1": "record-one"})
	if len(issues) == 0 || strings.Contains(strings.Join(issues, ";"), "PRIVATE_PATIENT_VALUE") {
		t.Fatal("diagnostics missing or private data leaked")
	}
	wire.Criteria = append(wire.Criteria, wire.Criteria...)
	for i := 0; i < 6; i++ {
		wire.Criteria = append(wire.Criteria, wire.Criteria...)
	}
	raw, _ = json.Marshal(wire)
	if len(evidenceBindingReview(raw, pack, sources, nil)) > 24 {
		t.Fatal("review diagnostics unbounded")
	}
}

// Feature-gated synthetic providers must honor the new output schema. This
// helper adds empty bindings only to their already-insufficient test clauses.
func withSyntheticBindings(content string, enabled bool) string {
	if !enabled {
		return content
	}
	var body map[string]any
	if json.Unmarshal([]byte(content), &body) != nil {
		return content
	}
	if clauses, ok := body["criteria"].([]any); ok {
		for _, value := range clauses {
			if clause, ok := value.(map[string]any); ok {
				if _, present := clause["evidence_bindings"]; !present {
					clause["evidence_bindings"] = []any{}
				}
			}
		}
	}
	raw, _ := json.Marshal(body)
	return string(raw)
}
