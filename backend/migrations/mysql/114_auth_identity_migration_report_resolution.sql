-- MySQL 方言版本（翻译自 ../114_auth_identity_migration_report_resolution.sql）
-- 翻译说明：
--   - ADD COLUMN IF NOT EXISTS → ADD COLUMN；CREATE INDEX IF NOT EXISTS → CREATE INDEX
--     （迁移执行器保证单次执行）。
--   - TIMESTAMPTZ NULL → DATETIME(3) NULL。
--   - TEXT NOT NULL DEFAULT '' → TEXT NOT NULL DEFAULT ('')（MySQL 8.0.13+ 表达式默认值，
--     TEXT 列不接受字面默认值，必须加括号）。

ALTER TABLE auth_identity_migration_reports
    ADD COLUMN resolved_at DATETIME(3) NULL;

ALTER TABLE auth_identity_migration_reports
    ADD COLUMN resolved_by_user_id BIGINT NULL;

ALTER TABLE auth_identity_migration_reports
    ADD COLUMN resolution_note TEXT NOT NULL DEFAULT ('');

CREATE INDEX idx_auth_identity_migration_reports_resolved_at
    ON auth_identity_migration_reports (resolved_at);
