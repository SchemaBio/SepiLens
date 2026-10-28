package evidence

import (
	"regexp"
	"strings"
)

var (
	reDigits  = regexp.MustCompile(`^\d{1,10}$`)
	reDOI     = regexp.MustCompile(`(?i)^10\.\d{4,9}/[-._;()/:A-Za-z0-9]+$`)
	reClinVar = regexp.MustCompile(`(?i)^(VCV\d+(\.\d+)?|RCV\d+(\.\d+)?|\d+)$`)
)

type SourceVerifier struct{}

func NewSourceVerifier() *SourceVerifier {
	return &SourceVerifier{}
}

func (sv *SourceVerifier) VerifySource(src *Source) {
	if src == nil {
		return
	}
	preservedStatus := src.ValidationStatus

	src.PMID = strings.TrimSpace(src.PMID)
	src.DOI = strings.TrimSpace(src.DOI)
	src.ClinVarID = strings.TrimSpace(src.ClinVarID)
	src.IdentifierValid = false

	switch src.Type {
	case SourceTypePubMed:
		if src.PMID != "" && reDigits.MatchString(src.PMID) {
			src.IdentifierValid = true
		} else if src.DOI != "" && reDOI.MatchString(src.DOI) {
			src.IdentifierValid = true
		} else {
			src.ValidationStatus = ValidationUnverified
		}
	case SourceTypeClinVar:
		if src.ClinVarID != "" && reClinVar.MatchString(src.ClinVarID) {
			src.IdentifierValid = true
		} else {
			src.ValidationStatus = ValidationUnverified
		}
	case SourceTypeClinGenDosage, SourceTypeCuratedSnapshot:
		src.IdentifierValid = src.DatasetVersion != ""
	case SourceTypeWebSearch:
		// Web search findings remain unverified until corroborated
		if src.ValidationStatus == "" {
			src.ValidationStatus = ValidationUnverified
		}
	default:
		if src.ValidationStatus == "" {
			src.ValidationStatus = ValidationUnverified
		}
	}
	src.ValidationStatus = ValidationUnverified
	if preservedStatus == ValidationConflict || preservedStatus == ValidationRejected {
		src.ValidationStatus = preservedStatus
		return
	}
	if src.IdentifierValid && src.RecordExists && src.ObjectMatched && src.MaterialVerified {
		src.ValidationStatus = ValidationVerified
	}
}

// EnforceEvidenceIntegrity guarantees medical invariant:
// An evidence item cannot claim "verified" unless all of its cited sources are verified.
// Items with unverified sources or no sources are downgraded from StatusMet to StatusProposedMet.
func (sv *SourceVerifier) EnforceEvidenceIntegrity(items []EvidenceItem, sources []Source) []EvidenceItem {
	verifiedSources := make(map[string]bool, len(sources))
	for _, s := range sources {
		if s.ValidationStatus == ValidationVerified && s.IdentifierValid && s.RecordExists && s.ObjectMatched && s.MaterialVerified {
			verifiedSources[s.ID] = true
		}
	}

	out := make([]EvidenceItem, len(items))
	for i, it := range items {
		hasAllVerified := len(it.SourceIDs) > 0
		for _, sID := range it.SourceIDs {
			if !verifiedSources[sID] {
				hasAllVerified = false
				break
			}
		}

		if hasAllVerified {
			it.SourceValidation = "verified"
		} else {
			it.SourceValidation = "unverified"
			it.ReviewRequired = true
			if it.Status == StatusMet {
				it.Status = StatusProposedMet
				it.MissingInformation = append(it.MissingInformation, "source verification pending")
			}
		}
		out[i] = it
	}
	return out
}

// DetectDuplicateCases inspects case counts across sources citing the same publication or registry ID
func (sv *SourceVerifier) DetectDuplicateCases(sources []Source) int {
	return UniqueSourceCaseCount(sources)
}
