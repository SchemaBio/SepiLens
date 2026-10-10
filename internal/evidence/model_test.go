package evidence_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/pzweuj/sepilens/internal/evidence"
)

func TestEvidence_ModelAndHashDeterminism(t *testing.T) {
	sources := []evidence.Source{
		{
			ID:                "SRC-PMID-25741868",
			Type:              evidence.SourceTypePubMed,
			Title:             "Standards and guidelines for the interpretation of sequence variants",
			PMID:              "25741868",
			RetrievedAt:       "2026-10-10T12:00:00Z",
			FullTextAvailable: true,
			ValidationStatus:  evidence.ValidationVerified,
			CaseCount:         1,
		},
	}

	items := []evidence.EvidenceItem{
		{
			ID:               "EVI-PVS1-001",
			EngineID:         "acmg_amp_2015",
			GuidelineVersion: "2015",
			RuleCode:         "PVS1",
			Status:           evidence.StatusProposedMet,
			Reason:           "Null variant in CFTR where LOF is known mechanism",
			SourceIDs:        []string{"SRC-PMID-25741868"},
			SourceValidation: "verified",
			ReviewRequired:   true,
		},
	}

	prov := evidence.ProvenanceMetadata{
		RetrievalMode:      "targeted",
		RetrievalTimestamp: "2026-10-10T12:00:00Z",
		TokensUsed:         120,
		RequestsCount:      1,
		DurationMS:         250,
	}

	hash1, err := evidence.ComputeSnapshotHash(sources, items, prov)
	if err != nil {
		t.Fatalf("failed to compute hash: %v", err)
	}

	hash2, err := evidence.ComputeSnapshotHash(sources, items, prov)
	if err != nil {
		t.Fatalf("failed to compute hash: %v", err)
	}

	if hash1 != hash2 {
		t.Fatalf("hash computation is not deterministic: %s vs %s", hash1, hash2)
	}

	// Change an item and ensure hash changes
	itemsMod := []evidence.EvidenceItem{
		{
			ID:               "EVI-PVS1-001",
			EngineID:         "acmg_amp_2015",
			GuidelineVersion: "2015",
			RuleCode:         "PVS1",
			Status:           evidence.StatusMet, // modified!
			Reason:           "Null variant in CFTR where LOF is known mechanism",
			SourceIDs:        []string{"SRC-PMID-25741868"},
			SourceValidation: "verified",
			ReviewRequired:   true,
		},
	}
	hashMod, _ := evidence.ComputeSnapshotHash(sources, itemsMod, prov)
	if hash1 == hashMod {
		t.Fatalf("hash must change when evidence item changes")
	}

	snapshot := evidence.EvidenceSnapshot{
		SnapshotID:   "snp-123",
		EvaluationID: "eval-456",
		TenantID:     "tenant-A",
		Hash:         hash1,
		Sources:      sources,
		Items:        items,
		Provenance:   prov,
		CreatedAt:    time.Now().UTC(),
	}

	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("failed to marshal snapshot: %v", err)
	}

	var decoded evidence.EvidenceSnapshot
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal snapshot: %v", err)
	}
	if decoded.SnapshotID != "snp-123" || decoded.Hash != hash1 || len(decoded.Sources) != 1 {
		t.Fatalf("unexpected decoded snapshot: %+v", decoded)
	}
}
