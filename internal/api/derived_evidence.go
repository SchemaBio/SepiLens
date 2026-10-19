package api

import (
	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/rules/acmg2015"
)

func derivedEvidence(d acmg2015.Derivation, engine string) []evidence.EvidenceItem {
	var out []evidence.EvidenceItem
	for _, c := range d.Candidates {
		out = append(out, evidence.EvidenceItem{ID: "derived-" + string(c.Code), EngineID: engine, RuleCode: string(c.Code), Status: evidence.EvidenceStatus(c.Status), SuggestedScore: string(c.Strength), Facts: c.Facts, Reason: c.Reason, SourceIDs: c.SourceIDs, ReviewRequired: true, SourceValidation: "unverified", MissingInformation: c.MissingInformation})
	}
	return out
}
