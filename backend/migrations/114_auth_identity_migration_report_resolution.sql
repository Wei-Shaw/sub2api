ALTER TABLE auth_identity_migration_reports
    ADD COLUMN IF NOT EXISTS resolved_at TIMESTAMPTZ NULL;

ALTER TABLE auth_identity_migration_reports
    ADD COLUMN IF NOT EXISTS resolved_by_user_id BIGINT NULL;

ALTER TABLE auth_identity_migration_reports
    ADD COLUMN IF NOT EXISTS resolution_note TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_auth_identity_migration_reports_resolved_at
    ON auth_identity_migration_reports (resolved_at);
