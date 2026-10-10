CREATE TABLE IF NOT EXISTS evidence_snapshots (
    snapshot_id TEXT PRIMARY KEY,
    evaluation_id TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    hash TEXT NOT NULL,
    sources_json TEXT NOT NULL,
    items_json TEXT NOT NULL,
    provenance_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_snapshots_eval_tenant ON evidence_snapshots(evaluation_id, tenant_id);
