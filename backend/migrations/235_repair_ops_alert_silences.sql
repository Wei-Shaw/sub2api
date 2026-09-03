-- Repair for 037_ops_alert_silences.sql.
--
-- 037 shipped with a goose Down block, but the migration runner
-- (internal/repository/migrations_runner.go) does not parse goose directives:
-- it executes the whole file in one transaction, so `DROP TABLE IF EXISTS
-- ops_alert_silences` ran immediately after the CREATE TABLE. Every install
-- therefore ended up without the table, while ops_repo_alerts.go still
-- INSERTs into / SELECTs from it at runtime.
--
-- 037 has been cleaned up for new installs; this migration restores the table
-- on databases that already recorded 037 as applied. Idempotent.

CREATE TABLE IF NOT EXISTS ops_alert_silences (
    id BIGSERIAL PRIMARY KEY,

    rule_id BIGINT NOT NULL,
    platform VARCHAR(64) NOT NULL,
    group_id BIGINT,
    region VARCHAR(64),

    until TIMESTAMPTZ NOT NULL,
    reason TEXT,

    created_by BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ops_alert_silences_lookup
    ON ops_alert_silences (rule_id, platform, group_id, region, until);
