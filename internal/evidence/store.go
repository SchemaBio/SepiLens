package evidence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
	db *sql.DB
}

func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

func (s *SQLiteStore) SaveSnapshot(ctx context.Context, snapshot EvidenceSnapshot) error {
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
	ON CONFLICT(snapshot_id) DO UPDATE SET
		hash = excluded.hash,
		sources_json = excluded.sources_json,
		items_json = excluded.items_json,
		provenance_json = excluded.provenance_json,
		created_at = excluded.created_at`

	_, err = s.db.ExecContext(ctx, query,
		snapshot.SnapshotID, snapshot.EvaluationID, snapshot.TenantID, snapshot.Hash,
		string(sourcesJSON), string(itemsJSON), string(provenanceJSON),
		createdAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("upsert snapshot: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetSnapshot(ctx context.Context, tenantID, evaluationID string) (*EvidenceSnapshot, error) {
	query := `SELECT snapshot_id, evaluation_id, tenant_id, hash,
		sources_json, items_json, provenance_json, created_at
		FROM evidence_snapshots
		WHERE tenant_id = ? AND evaluation_id = ?
		ORDER BY created_at DESC LIMIT 1`

	var snp EvidenceSnapshot
	var sourcesStr, itemsStr, provStr, createdStr string

	err := s.db.QueryRowContext(ctx, query, tenantID, evaluationID).Scan(
		&snp.SnapshotID, &snp.EvaluationID, &snp.TenantID, &snp.Hash,
		&sourcesStr, &itemsStr, &provStr, &createdStr,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSnapshotNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query snapshot: %w", err)
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

	return &snp, nil
}
