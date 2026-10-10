package evidence_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/storage/sqlite"
)

func TestEvidence_StoreAndTenantIsolation(t *testing.T) {
	dir := t.TempDir()
	db, err := sqlite.Open(filepath.Join(dir, "evidence_test.db"))
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	defer db.Close()

	store := evidence.NewSQLiteStore(db)
	ctx := context.Background()

	snp := evidence.EvidenceSnapshot{
		SnapshotID:   "snp-001",
		EvaluationID: "eval-100",
		TenantID:     "tenant-A",
		Hash:         "hash-123456",
		Sources: []evidence.Source{
			{
				ID:               "SRC-01",
				Type:             evidence.SourceTypePubMed,
				Title:            "Study on CFTR deltaF508",
				PMID:             "12345678",
				ValidationStatus: evidence.ValidationVerified,
			},
		},
		Items: []evidence.EvidenceItem{
			{
				ID:       "EVI-01",
				EngineID: "acmg_amp_2015",
				RuleCode: "PS1",
				Status:   evidence.StatusMet,
			},
		},
		Provenance: evidence.ProvenanceMetadata{
			RetrievalMode: "targeted",
			TokensUsed:    100,
		},
		CreatedAt: time.Now().UTC(),
	}

	if err := store.SaveSnapshot(ctx, snp); err != nil {
		t.Fatalf("failed to save snapshot: %v", err)
	}

	// Read by correct tenant
	fetched, err := store.GetSnapshot(ctx, "tenant-A", "eval-100")
	if err != nil {
		t.Fatalf("failed to fetch snapshot: %v", err)
	}
	if fetched.SnapshotID != "snp-001" || fetched.Hash != "hash-123456" || len(fetched.Sources) != 1 {
		t.Fatalf("unexpected fetched snapshot: %+v", fetched)
	}

	// Read by other tenant -> ErrSnapshotNotFound
	_, errTenantB := store.GetSnapshot(ctx, "tenant-B", "eval-100")
	if !errors.Is(errTenantB, evidence.ErrSnapshotNotFound) {
		t.Fatalf("expected ErrSnapshotNotFound for other tenant, got %v", errTenantB)
	}
}
