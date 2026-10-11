UPDATE jobs SET result_kind='legacy_unverified';
UPDATE jobs SET idempotency_key=NULL WHERE idempotency_key IS NOT NULL AND idempotency_key<>'' AND rowid NOT IN (SELECT MIN(rowid) FROM jobs WHERE idempotency_key IS NOT NULL AND idempotency_key<>'' GROUP BY tenant_id,idempotency_key);
CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_unique_idempotency ON jobs(tenant_id,idempotency_key) WHERE idempotency_key IS NOT NULL AND idempotency_key<>'';
CREATE UNIQUE INDEX IF NOT EXISTS idx_events_unique_sequence ON job_events(job_id,seq);
CREATE TABLE IF NOT EXISTS work_inputs(job_id TEXT PRIMARY KEY,tenant_id TEXT NOT NULL,payload TEXT NOT NULL,owner TEXT,lease_until TEXT,external_started INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS provider_profiles(tenant_id TEXT NOT NULL,profile_id TEXT NOT NULL,version INTEGER NOT NULL,config TEXT NOT NULL,created_at TEXT NOT NULL,PRIMARY KEY(tenant_id,profile_id,version));
CREATE TABLE IF NOT EXISTS profile_deleted(tenant_id TEXT NOT NULL,profile_id TEXT NOT NULL,PRIMARY KEY(tenant_id,profile_id));
CREATE TABLE IF NOT EXISTS audit_versions(tenant_id TEXT NOT NULL,evaluation_id TEXT NOT NULL,version INTEGER NOT NULL,payload TEXT NOT NULL,created_at TEXT NOT NULL,PRIMARY KEY(tenant_id,evaluation_id,version));
