-- Upstream balance monitor configurations and the latest successful probe snapshot.
-- API keys are encrypted by the service before they reach this table.

CREATE TABLE IF NOT EXISTS upstream_balance_monitors (
    id                         BIGSERIAL PRIMARY KEY,
    name                       VARCHAR(100) NOT NULL,
    type                       VARCHAR(16)  NOT NULL,
    base_url                   VARCHAR(500) NOT NULL,
    api_key_encrypted          TEXT         NOT NULL,
    enabled                    BOOLEAN      NOT NULL DEFAULT TRUE,
    display_order              INT          NOT NULL DEFAULT 0,
    probe_interval_minutes     INT          NOT NULL DEFAULT 30,
    low_balance_threshold_usd  DOUBLE PRECISION NOT NULL DEFAULT 10,
    last_probe_at              TIMESTAMPTZ,
    last_probe_status          VARCHAR(16)  NOT NULL DEFAULT 'pending',
    last_probe_error           TEXT,
    snapshot_data              JSONB        NOT NULL DEFAULT '{}'::jsonb,
    next_probe_at              TIMESTAMPTZ,
    failure_count              INT          NOT NULL DEFAULT 0,
    created_at                 TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at                 TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT upstream_balance_monitors_type_check
        CHECK (type IN ('sub2api', 'newapi')),
    CONSTRAINT upstream_balance_monitors_interval_check
        CHECK (probe_interval_minutes BETWEEN 5 AND 1440),
    CONSTRAINT upstream_balance_monitors_threshold_check
        CHECK (low_balance_threshold_usd >= 0),
    CONSTRAINT upstream_balance_monitors_failure_count_check
        CHECK (failure_count >= 0)
);

CREATE INDEX IF NOT EXISTS idx_upstream_balance_monitors_enabled_next_probe
    ON upstream_balance_monitors (enabled, next_probe_at);
CREATE INDEX IF NOT EXISTS idx_upstream_balance_monitors_display_order_id
    ON upstream_balance_monitors (display_order, id);
