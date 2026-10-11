package api

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/pzweuj/sepilens/internal/evidence"
)

func (s *Server) auditHistory(ctx context.Context, tenant, id string) ([]any, []evidence.EvidenceSnapshot, error) {
	if s.queue == nil {
		return nil, nil, errors.New("encrypted audit unavailable")
	}
	rows, e := s.db.QueryContext(ctx, `SELECT version,payload FROM audit_versions WHERE tenant_id=? AND evaluation_id=? ORDER BY version`, tenant, id)
	if e != nil {
		return nil, nil, e
	}
	var history []any
	for rows.Next() {
		var version int
		var enc string
		if e = rows.Scan(&version, &enc); e != nil {
			rows.Close()
			return nil, nil, e
		}
		plain, err := s.queue.Cipher.Open(enc, tenant+":"+id+":result")
		if err != nil {
			rows.Close()
			return nil, nil, err
		}
		var payload any
		if json.Unmarshal([]byte(plain), &payload) != nil {
			rows.Close()
			return nil, nil, errors.New("history integrity failed")
		}
		history = append(history, map[string]any{"revision": version, "result": payload})
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, nil, e
	}
	snapshots, e := s.queue.Evidence.History(ctx, tenant, id)
	return history, snapshots, e
}
