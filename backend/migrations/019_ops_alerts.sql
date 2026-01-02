-- Ops alert rules and events

CREATE TABLE IF NOT EXISTS ops_alert_rules (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(128) NOT NULL,
    description TEXT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    metric_type VARCHAR(64) NOT NULL,
    operator VARCHAR(8) NOT NULL,
    threshold DOUBLE PRECISION NOT NULL,
    window_minutes INT NOT NULL DEFAULT 1,
    sustained_minutes INT NOT NULL DEFAULT 1,
    severity VARCHAR(4) NOT NULL DEFAULT 'P1',
    notify_email BOOLEAN NOT NULL DEFAULT FALSE,
    notify_webhook BOOLEAN NOT NULL DEFAULT FALSE,
    webhook_url TEXT,
    cooldown_minutes INT NOT NULL DEFAULT 10,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ops_alert_rules_enabled ON ops_alert_rules (enabled);
CREATE INDEX IF NOT EXISTS idx_ops_alert_rules_metric ON ops_alert_rules (metric_type, window_minutes);

CREATE TABLE IF NOT EXISTS ops_alert_events (
    id BIGSERIAL PRIMARY KEY,
    rule_id BIGINT NOT NULL REFERENCES ops_alert_rules(id) ON DELETE CASCADE,
    severity VARCHAR(4) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'firing',
    title VARCHAR(200),
    description TEXT,
    metric_value DOUBLE PRECISION,
    threshold_value DOUBLE PRECISION,
    fired_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    email_sent BOOLEAN NOT NULL DEFAULT FALSE,
    webhook_sent BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ops_alert_events_rule_status ON ops_alert_events (rule_id, status);
CREATE INDEX IF NOT EXISTS idx_ops_alert_events_fired_at ON ops_alert_events (fired_at DESC);
