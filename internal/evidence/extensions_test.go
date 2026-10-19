package evidence_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pzweuj/sepilens/internal/evidence"
	"github.com/pzweuj/sepilens/internal/secure"
	"github.com/pzweuj/sepilens/internal/storage/sqlite"
)

func TestSnapshotHashVersionsAndTampering(t *testing.T) {
	s := evidence.EvidenceSnapshot{}
	v1, _ := evidence.ComputeSnapshotHash(s.Sources, s.Items, s.Provenance)
	if v1 != "9f26c9ecbd62aa0f80da7f1b4f04c0cd823fc1ec8558271d6869f959a135a8a8" {
		t.Fatal("legacy canonical bytes changed", v1)
	}
	h, e := evidence.SnapshotHash(s)
	if e != nil || h != v1 {
		t.Fatal("v1 changed")
	}
	s.Annotations = json.RawMessage(`{"marker":1}`)
	if _, e = evidence.SnapshotHash(s); e == nil {
		t.Fatal("v1 extension downgrade accepted")
	}
	s.HashVersion = "v2"
	h, e = evidence.SnapshotHash(s)
	if e != nil {
		t.Fatal(e)
	}
	s.Annotations = json.RawMessage("{\n  \"marker\":1\n}")
	same, _ := evidence.SnapshotHash(s)
	if same != h {
		t.Fatal("whitespace changed v2 hash")
	}
	s.Annotations = json.RawMessage(`{"marker":2}`)
	changed, _ := evidence.SnapshotHash(s)
	if changed == h {
		t.Fatal("extension tampering ignored")
	}
	s.HashVersion = "v3"
	if _, e = evidence.SnapshotHash(s); e == nil {
		t.Fatal("unknown version accepted")
	}
}
func TestExtensionsEncryptedHistoryAndKeyRotation(t *testing.T) {
	db, e := sqlite.Open(filepath.Join(t.TempDir(), "s.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	key, _ := secure.New("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	ctx := context.Background()
	store := evidence.NewEncryptedStore(db, key)
	s := evidence.EvidenceSnapshot{SnapshotID: "one", EvaluationID: "job", TenantID: "tenant"}
	s.HashVersion = "v2"
	s.Annotations = json.RawMessage(`{"marker":"private-annotation"}`)
	s.Hash, _ = evidence.SnapshotHash(s)
	if e = store.SaveSnapshot(ctx, s); e != nil {
		t.Fatal(e)
	}
	var enc string
	if e = db.QueryRow("SELECT extensions_json FROM evidence_snapshots").Scan(&enc); e != nil || !strings.HasPrefix(enc, "enc1:") || strings.Contains(enc, "private-annotation") {
		t.Fatal("extensions not encrypted", e)
	}
	got, e := store.GetSnapshot(ctx, "tenant", "job")
	if e != nil || string(got.Annotations) != string(s.Annotations) {
		t.Fatal(got, e)
	}
	if _, e = store.GetSnapshot(ctx, "other", "job"); e == nil {
		t.Fatal("cross tenant read")
	}
	newKey, _ := secure.New("bm5ubm5ubm5ubm5ubm5ubm5ubm5ubm5ubm5ubm5ubm4=")
	if e = secure.Reencrypt(ctx, db, key, newKey, false); e != nil {
		t.Fatal(e)
	}
	if _, e = evidence.NewEncryptedStore(db, newKey).GetSnapshot(ctx, "tenant", "job"); e != nil {
		t.Fatal("extension lost during key rotation", e)
	}
}
