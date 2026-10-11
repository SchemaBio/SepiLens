ALTER TABLE evidence_snapshots ADD COLUMN legacy_unverified INTEGER NOT NULL DEFAULT 0;
UPDATE evidence_snapshots SET legacy_unverified=1 WHERE evaluation_id IN (SELECT job_id FROM jobs WHERE result_kind='legacy_unverified');
