package sqlite

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyMigrationPreservesDuplicateHistory(t *testing.T) {
	p := filepath.Join(t.TempDir(), "legacy.db")
	db, e := sql.Open("sqlite", p)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(initSQL + evidenceSQL); e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{"old-a", "old-b"} {
		if _, e = db.Exec(`INSERT INTO jobs(job_id,tenant_id,engine_id,status,result_kind,request_hash,idempotency_key,stage,created_at,updated_at) VALUES(?, 'T','acmg_amp_2015','succeeded','production','hash','duplicate','done','2000-01-01','2000-01-01')`, id); e != nil {
			t.Fatal(e)
		}
	}
	db.Close()
	migrated, e := Open(p)
	if e != nil {
		t.Fatal(e)
	}
	defer migrated.Close()
	var count, active, legacy int
	if e = migrated.QueryRow(`SELECT COUNT(*),COUNT(idempotency_key),SUM(result_kind='legacy_unverified') FROM jobs`).Scan(&count, &active, &legacy); e != nil {
		t.Fatal(e)
	}
	if count != 2 || active != 1 || legacy != 2 {
		t.Fatal("migration lost or upgraded history", count, active, legacy)
	}
	if _, e = os.Stat(p + ".pre-v3.bak"); e != nil {
		t.Fatal("no pre migration backup", e)
	}
	var version int
	if e = migrated.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); e != nil || version != 7 {
		t.Fatal("schema migration incomplete", version, e)
	}
}
