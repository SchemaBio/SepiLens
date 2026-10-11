package evidence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pzweuj/sepilens/internal/secure"
	"time"
)

var (
	ErrSnapshotNotFound = errors.New("evidence snapshot not found")
)

type Store interface {
	SaveSnapshot(ctx context.Context, snapshot EvidenceSnapshot) error
	GetSnapshot(ctx context.Context, tenantID, evaluationID string) (*EvidenceSnapshot, error)
}

type SQLiteStore struct {
	db     *sql.DB
	cipher *secure.Cipher
}

func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}
func NewEncryptedStore(db *sql.DB, c *secure.Cipher) *SQLiteStore {
	return &SQLiteStore{db: db, cipher: c}
}

type Executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (s *SQLiteStore) SaveSnapshot(ctx context.Context, snapshot EvidenceSnapshot) error {
	return s.SaveTx(ctx, s.db, snapshot)
}
func (s *SQLiteStore) SaveTx(ctx context.Context, exec Executor, snapshot EvidenceSnapshot) error {
	h, err := ComputeSnapshotHash(snapshot.Sources, snapshot.Items, snapshot.Provenance)
	if err != nil || h != snapshot.Hash {
		return errors.New("evidence snapshot hash mismatch")
	}
	sourcesJSON, err := json.Marshal(snapshot.Sources)
	if err != nil {
		return fmt.Errorf("marshal sources: %w", err)
	}
	itemsJSON, err := json.Marshal(snapshot.Items)
	if err != nil {
		return fmt.Errorf("marshal items: %w", err)
	}
	provenanceJSON, err := json.Marshal(snapshot.Provenance)
	if err != nil {
		return fmt.Errorf("marshal provenance: %w", err)
	}

	createdAt := snapshot.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	query := `INSERT INTO evidence_snapshots (
		snapshot_id, evaluation_id, tenant_id, hash,
		sources_json, items_json, provenance_json, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	a, b, c := string(sourcesJSON), string(itemsJSON), string(provenanceJSON)
	if s.cipher != nil {
		binding := snapshot.TenantID + ":" + snapshot.SnapshotID
		a, err = s.cipher.Seal(a, binding+":sources")
		if err != nil {
			return err
		}
		b, err = s.cipher.Seal(b, binding+":items")
		if err != nil {
			return err
		}
		c, err = s.cipher.Seal(c, binding+":provenance")
		if err != nil {
			return err
		}
	}

	_, err = exec.ExecContext(ctx, query,
		snapshot.SnapshotID, snapshot.EvaluationID, snapshot.TenantID, snapshot.Hash,
		a, b, c,
		createdAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("upsert snapshot: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetSnapshot(ctx context.Context, tenantID, evaluationID string) (*EvidenceSnapshot, error) {
	return s.getSnapshot(ctx, tenantID, evaluationID, "")
}
func (s *SQLiteStore) getSnapshot(ctx context.Context, tenantID, evaluationID, snapshotID string) (*EvidenceSnapshot, error) {
	query := `SELECT legacy_unverified,snapshot_id, evaluation_id, tenant_id, hash,
		sources_json, items_json, provenance_json, created_at
		FROM evidence_snapshots
		WHERE tenant_id = ? AND evaluation_id = ? AND (?='' OR snapshot_id=?)
		ORDER BY created_at DESC LIMIT 1`

	var snp EvidenceSnapshot
	var sourcesStr, itemsStr, provStr, createdStr string

	err := s.db.QueryRowContext(ctx, query, tenantID, evaluationID, snapshotID, snapshotID).Scan(
		&snp.LegacyUnverified,
		&snp.SnapshotID, &snp.EvaluationID, &snp.TenantID, &snp.Hash,
		&sourcesStr, &itemsStr, &provStr, &createdStr,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSnapshotNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query snapshot: %w", err)
	}
	if s.cipher != nil {
		binding := tenantID + ":" + snp.SnapshotID
		sourcesStr, err = s.cipher.Open(sourcesStr, binding+":sources")
		if err != nil {
			return nil, err
		}
		itemsStr, err = s.cipher.Open(itemsStr, binding+":items")
		if err != nil {
			return nil, err
		}
		provStr, err = s.cipher.Open(provStr, binding+":provenance")
		if err != nil {
			return nil, err
		}
	}

	if err := json.Unmarshal([]byte(sourcesStr), &snp.Sources); err != nil {
		return nil, fmt.Errorf("unmarshal sources: %w", err)
	}
	if err := json.Unmarshal([]byte(itemsStr), &snp.Items); err != nil {
		return nil, fmt.Errorf("unmarshal items: %w", err)
	}
	if err := json.Unmarshal([]byte(provStr), &snp.Provenance); err != nil {
		return nil, fmt.Errorf("unmarshal provenance: %w", err)
	}
	snp.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
	if snp.LegacyUnverified {
		for i := range snp.Sources {
			snp.Sources[i].ValidationStatus = ValidationUnverified
			snp.Sources[i].IdentifierValid = false
			snp.Sources[i].RecordExists = false
			snp.Sources[i].ObjectMatched = false
			snp.Sources[i].MaterialVerified = false
		}
		for i := range snp.Items {
			snp.Items[i].Status = StatusPendingReview
			snp.Items[i].SourceValidation = "unverified"
			snp.Items[i].ScoreApplied = nil
			snp.Items[i].ReviewRequired = true
		}
		return &snp, nil
	}
	hash, err := ComputeSnapshotHash(snp.Sources, snp.Items, snp.Provenance)
	if err != nil || hash != snp.Hash {
		return nil, errors.New("evidence snapshot integrity failed")
	}

	return &snp, nil
}
func (s *SQLiteStore) History(ctx context.Context, tenant, evaluation string) ([]EvidenceSnapshot, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT snapshot_id FROM evidence_snapshots WHERE tenant_id=? AND evaluation_id=? ORDER BY created_at,snapshot_id`, tenant, evaluation)
	if e != nil {
		return nil, e
	}
	var ids []string
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return nil, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	var out []EvidenceSnapshot
	for _, id := range ids {
		snp, e := s.getSnapshot(ctx, tenant, evaluation, id)
		if e != nil {
			return nil, e
		}
		out = append(out, *snp)
	}
	return out, nil
}
