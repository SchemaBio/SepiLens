package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

//go:embed migrations/001_init.sql
var initSQL string

//go:embed migrations/002_evidence.sql
var evidenceSQL string

//go:embed migrations/003_runtime.sql
var runtimeSQL string

//go:embed migrations/004_encryption.sql
var encryptionSQL string

//go:embed migrations/005_review_versions.sql
var reviewVersionsSQL string

//go:embed migrations/006_legacy_evidence.sql
var legacyEvidenceSQL string

//go:embed migrations/007_snapshot_extensions.sql
var snapshotExtensionsSQL string

func Open(dbPath string) (*sql.DB, error) {
	// Inspect existing files before enabling WAL, which can alter a legacy header.
	if st, err := os.Stat(dbPath); err == nil && st.Size() > 0 && dbPath != ":memory:" {
		abs, e := filepath.Abs(dbPath)
		if e != nil {
			return nil, e
		}
		path := filepath.ToSlash(abs)
		if len(path) > 1 && path[1] == ':' {
			path = "/" + path
		}
		u := url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}
		probe, e := sql.Open("sqlite", u.String())
		if e != nil {
			return nil, e
		}
		var id int
		e = probe.QueryRow(`PRAGMA application_id`).Scan(&id)
		probe.Close()
		if e != nil || id != 0x53455032 {
			return nil, fmt.Errorf("unsupported legacy business database: specify a new data directory; old files were not modified")
		}
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create sqlite dir: %w", err)
		}
	}

	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	db.SetMaxOpenConns(1) // SQLite single writer safety

	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	var applicationID, tables int
	if err := db.QueryRow(`PRAGMA application_id`).Scan(&applicationID); err != nil {
		db.Close()
		return nil, err
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`).Scan(&tables); err != nil {
		db.Close()
		return nil, err
	}
	const formatID = 0x53455032 // SEP2: external-evidence business storage.
	if (tables > 0 && applicationID != formatID) || (applicationID != 0 && applicationID != formatID) {
		db.Close()
		return nil, fmt.Errorf("unsupported legacy business database: specify a new data directory; old files were not modified")
	}
	if applicationID == 0 {
		if _, err := db.Exec(`PRAGMA application_id=1397051442`); err != nil {
			db.Close()
			return nil, err
		}
	}

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY,applied_at TEXT NOT NULL)`); err != nil {
		db.Close()
		return nil, err
	}
	var current int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&current); err != nil {
		db.Close()
		return nil, err
	}
	for i, migration := range []string{initSQL, evidenceSQL, runtimeSQL, encryptionSQL, reviewVersionsSQL, legacyEvidenceSQL, snapshotExtensionsSQL} {
		if i+1 <= current {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			db.Close()
			return nil, err
		}
		if _, err := tx.Exec(migration); err != nil {
			_ = tx.Rollback()
			_ = db.Close()
			return nil, fmt.Errorf("run sqlite migration: %w", err)
		}
		if _, err = tx.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES (?,datetime('now'))`, i+1); err != nil {
			tx.Rollback()
			db.Close()
			return nil, err
		}
		if err = tx.Commit(); err != nil {
			db.Close()
			return nil, err
		}
	}

	return db, nil
}
