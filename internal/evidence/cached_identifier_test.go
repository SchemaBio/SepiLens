package evidence

import "testing"

func TestMalformedIdentifierCannotKeepPreviouslyVerifiedFlag(t *testing.T) {
	for _, src := range []Source{
		{ID: "bad", Type: SourceTypePubMed, PMID: "not-a-record", IdentifierValid: true, RecordExists: true, ObjectMatched: true, MaterialVerified: true, ValidationStatus: ValidationVerified},
		{ID: "bad", Type: SourceTypeClinVar, ClinVarID: "VCVinvalid", IdentifierValid: true, RecordExists: true, ObjectMatched: true, MaterialVerified: true, ValidationStatus: ValidationVerified},
	} {
		NewSourceVerifier().VerifySource(&src)
		if src.IdentifierValid || src.ValidationStatus == ValidationVerified {
			t.Fatal("invalid record retained trust")
		}
	}
}
