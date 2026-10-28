package evidence

import (
	"reflect"
	"testing"
)

func TestPublicationIdentityLinksPMIDDOIAndKeepsNamespaces(t *testing.T) {
	sources := []Source{{ID: "one", PMID: "00123", DOI: "https://doi.org/10.1234/ABC"}, {ID: "two", DOI: "doi:10.1234/abc"}, {ID: "three", PMID: "123"}, {ID: "different", Type: SourceTypeClinVar, ClinVarID: "123"}}
	groups := GroupSourceDocuments(sources)
	if len(groups) != 2 || len(groups[0].Records) != 3 {
		t.Fatal("publication bridge or namespace isolation failed", groups)
	}
	for _, source := range groups[0].Records {
		if source.ID == "different" {
			t.Fatal("numeric ClinVar ID collided with PMID")
		}
	}
	if len(GroupSourceDocuments([]Source{{ID: "x", PMID: "123", URL: "https://example.org/"}, {ID: "y", PMID: "456", URL: "https://example.org/"}})) != 2 {
		t.Fatal("landing page merged distinct publications")
	}
}
func TestRecordMergePreservesCitationsAndNeverCombinesTrust(t *testing.T) {
	first := Source{ID: "original", PMID: "123", Snippet: "first", ObjectMatched: true, MaterialVerified: false}
	next := Source{ID: "original", PMID: "123", Snippet: "different", ObjectMatched: false, MaterialVerified: true}
	merged := MergeSourceRecords([]Source{first}, []Source{next})
	if len(merged) != 2 || !reflect.DeepEqual(merged[0], first) || merged[1].ID == "original" || merged[1].ObjectMatched {
		t.Fatal("records lost or trust combined")
	}
	if len(MergeSourceRecords(merged, []Source{next})) != 2 {
		t.Fatal("same variant added twice")
	}
	later := first
	later.RetrievedAt = "later"
	if len(MergeSourceRecords([]Source{first}, []Source{later})) != 1 {
		t.Fatal("timestamp created a duplicate")
	}
}
func TestCaseMetadataUsesExplicitIdentitiesAndConservativeCounts(t *testing.T) {
	sources := []Source{{ID: "a", PMID: "123", DOI: "10.1234/test", CaseCount: 3}, {ID: "b", DOI: "10.1234/test", CaseCount: 5}, {ID: "c", PMID: "456", CaseCount: 2}}
	if UniqueSourceCaseCount(sources) != 7 {
		t.Fatal("same paper counted twice")
	}
	sources[0].CaseIDs = []string{"A", "A", "B"}
	sources[1].CaseIDs = []string{"B", "C"}
	if UniqueSourceCaseCount(sources) != 5 {
		t.Fatal("explicit identities double counted")
	}
	if UniqueSourceCaseCount([]Source{{ID: "negative", CaseCount: -9}}) != 0 {
		t.Fatal("negative count accepted")
	}
	if PublicDocumentKey(DocumentGroup{Records: []Source{{ID: "PRIVATE_PATIENT_ID", URL: "https://example.org/?patient=PRIVATE_PATIENT_ID"}}}) != "" {
		t.Fatal("private identity entered public grouping key")
	}
}
func TestVerificationPreservesExplicitConflictAndRejection(t *testing.T) {
	for _, status := range []SourceValidation{ValidationConflict, ValidationRejected} {
		source := Source{Type: SourceTypePubMed, PMID: "123", RecordExists: true, ObjectMatched: true, MaterialVerified: true, ValidationStatus: status}
		NewSourceVerifier().VerifySource(&source)
		if source.ValidationStatus != status {
			t.Fatal("negative status overwritten")
		}
	}
}
