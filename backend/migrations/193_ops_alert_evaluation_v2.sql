-- Alert-evaluation v2 is rolled out additively. Columns added to existing
-- tables remain nullable so old and new application images can write during a
-- blue-green deployment without a table rewrite or rollback incompatibility.
ALTER TABLE ops_alert_rules
    ADD COLUMN IF NOT EXISTS incident_family VARCHAR(64);
ALTER TABLE ops_alert_rules
    ADD COLUMN IF NOT EXISTS minimum_samples INTEGER;
ALTER TABLE ops_alert_rules
    ADD COLUMN IF NOT EXISTS minimum_bad_count INTEGER;
ALTER TABLE ops_alert_rules
    ADD COLUMN IF NOT EXISTS recovery_operator VARCHAR(8);
ALTER TABLE ops_alert_rules
    ADD COLUMN IF NOT EXISTS recovery_threshold DOUBLE PRECISION;
ALTER TABLE ops_alert_rules
    ADD COLUMN IF NOT EXISTS recovery_sustained_minutes INTEGER;
ALTER TABLE ops_alert_rules
    ADD COLUMN IF NOT EXISTS shadow_mode BOOLEAN;

ALTER TABLE ops_alert_events
    ADD COLUMN IF NOT EXISTS email_queued BOOLEAN;

CREATE TABLE ops_alert_rule_evaluations (
    id                  BIGSERIAL PRIMARY KEY,
    rule_id             BIGINT NOT NULL,
    evaluated_at        TIMESTAMPTZ NOT NULL,
    window_start        TIMESTAMPTZ NOT NULL,
    window_end          TIMESTAMPTZ NOT NULL,
    status              VARCHAR(32) NOT NULL,
    breached            BOOLEAN NOT NULL DEFAULT FALSE,
    metric_value        DOUBLE PRECISION,
    threshold_value     DOUBLE PRECISION,
    sample_count        BIGINT NOT NULL DEFAULT 0,
    bad_count           BIGINT NOT NULL DEFAULT 0,
    data_as_of          TIMESTAMPTZ,
    error_code          VARCHAR(64),
    error_message       TEXT,
    evaluator_version   VARCHAR(32) NOT NULL DEFAULT 'v2',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ops_alert_rule_evaluations_status_check CHECK (
        status IN ('ok', 'breached', 'no_data', 'stale', 'error', 'unsupported', 'disabled', 'shadow')
    )
);

CREATE INDEX idx_ops_alert_rule_evaluations_rule_time
    ON ops_alert_rule_evaluations (rule_id, evaluated_at DESC, id DESC);

CREATE INDEX idx_ops_alert_rule_evaluations_time
    ON ops_alert_rule_evaluations (evaluated_at DESC, id DESC);

CREATE TABLE ops_alert_rule_states (
    rule_id                 BIGINT PRIMARY KEY,
    last_evaluated_at       TIMESTAMPTZ,
    consecutive_breaches   INTEGER NOT NULL DEFAULT 0,
    consecutive_recoveries INTEGER NOT NULL DEFAULT 0,
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
