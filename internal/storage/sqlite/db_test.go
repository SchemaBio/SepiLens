package sqlite

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyDatabaseRejectedWithoutModification(t *testing.T) {
	p := filepath.Join(t.TempDir(), "legacy.db")
	db, e := sql.Open("sqlite", p)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(initSQL + evidenceSQL); e != nil {
		t.Fatal(e)
	}
	db.Close()
	before, _ := os.ReadFile(p)
	if db, e = Open(p); e == nil {
		db.Close()
		t.Fatal("legacy accepted")
	}
	after, _ := os.ReadFile(p)
	if string(before) != string(after) {
		t.Fatal("legacy database changed")
	}
}
func TestV2StorageCanReopen(t *testing.T) {
	p := filepath.Join(t.TempDir(), "v2.db")
	db, e := Open(p)
	if e != nil {
		t.Fatal(e)
	}
	db.Close()
	db, e = Open(p)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var id int
	if e = db.QueryRow(`PRAGMA application_id`).Scan(&id); e != nil || id != 0x53455032 {
		t.Fatal(id, e)
	}
}
