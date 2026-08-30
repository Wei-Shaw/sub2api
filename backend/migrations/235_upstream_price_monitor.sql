-- Model-level upstream price monitoring. API keys remain owned by accounts;
-- these tables persist only sanitized cumulative counters and reconciliation
-- evidence.

CREATE TABLE IF NOT EXISTS upstream_price_monitor_config (
    id SMALLINT PRIMARY KEY DEFAULT 1,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    mode VARCHAR(24) NOT NULL DEFAULT 'observe',
    interval_minutes INTEGER NOT NULL DEFAULT 15,
    markup NUMERIC(12, 6) NOT NULL DEFAULT 1.200000,
    display_multiplier_decimals SMALLINT NOT NULL DEFAULT 3,
    account_ids BIGINT[] NOT NULL DEFAULT '{}',
    channel_ids BIGINT[] NOT NULL DEFAULT '{}',
    domestic_models TEXT[] NOT NULL DEFAULT ARRAY[
        'deepseek-v4-flash-0731', 'deepseek-v4-pro-0813', 'deepseek-v4-flash-vision-exp',
        'kimi-k2.6', 'kimi-k2.7-code', 'kimi-k3',
        'glm-5.1', 'glm-5.2', 'glm-5.3', 'glm-5.3-flash',
        'MiniMax-M2.7', 'MiniMax-M2.7-highspeed', 'MiniMax-M3',
        'qwen3.7-max', 'qwen3.8-max', 'mimo-v2.5', 'mimo-v2.5-pro', 'hy3'
    ],
    passive_sample_max_age_minutes INTEGER NOT NULL DEFAULT 60,
    active_probe_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT upstream_price_monitor_config_singleton CHECK (id = 1),
    CONSTRAINT upstream_price_monitor_config_mode_check CHECK (mode IN ('observe', 'auto_apply')),
    CONSTRAINT upstream_price_monitor_config_interval_check CHECK (interval_minutes BETWEEN 5 AND 1440),
    CONSTRAINT upstream_price_monitor_config_markup_check CHECK (markup >= 1 AND markup <= 100),
    CONSTRAINT upstream_price_monitor_config_decimals_check CHECK (display_multiplier_decimals BETWEEN 0 AND 6),
    CONSTRAINT upstream_price_monitor_config_max_age_check CHECK (passive_sample_max_age_minutes BETWEEN 15 AND 10080)
);

INSERT INTO upstream_price_monitor_config (id)
VALUES (1)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS upstream_price_monitor_runs (
    id BIGSERIAL PRIMARY KEY,
    trigger VARCHAR(24) NOT NULL,
    status VARCHAR(24) NOT NULL DEFAULT 'running',
    mode VARCHAR(24) NOT NULL DEFAULT 'observe',
    dry_run BOOLEAN NOT NULL DEFAULT TRUE,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    matched_models INTEGER NOT NULL DEFAULT 0,
    mismatched_models INTEGER NOT NULL DEFAULT 0,
    probed_models INTEGER NOT NULL DEFAULT 0,
    probe_cost NUMERIC(20, 10) NOT NULL DEFAULT 0,
    snapshot_hash CHAR(64) NOT NULL DEFAULT '',
    summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    error TEXT NOT NULL DEFAULT '',
    applied_at TIMESTAMPTZ,
    rollback_available BOOLEAN NOT NULL DEFAULT FALSE,
    rollback_snapshot JSONB,
    CONSTRAINT upstream_price_monitor_runs_trigger_check CHECK (trigger IN ('scheduled', 'manual', 'active_probe')),
    CONSTRAINT upstream_price_monitor_runs_status_check CHECK (status IN ('running', 'completed', 'partial', 'failed')),
    CONSTRAINT upstream_price_monitor_runs_mode_check CHECK (mode IN ('observe', 'auto_apply')),
    CONSTRAINT upstream_price_monitor_runs_counts_check CHECK (matched_models >= 0 AND mismatched_models >= 0 AND probed_models >= 0),
    CONSTRAINT upstream_price_monitor_runs_cost_check CHECK (probe_cost >= 0)
);

CREATE INDEX IF NOT EXISTS idx_upstream_price_monitor_runs_started
    ON upstream_price_monitor_runs (started_at DESC, id DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_upstream_price_monitor_one_running
    ON upstream_price_monitor_runs ((1)) WHERE status = 'running';

CREATE TABLE IF NOT EXISTS upstream_price_monitor_model_scan_state (
    id SMALLINT PRIMARY KEY DEFAULT 1,
    revision BIGINT NOT NULL DEFAULT 0,
    discovery_complete BOOLEAN NOT NULL DEFAULT FALSE,
    last_scan_at TIMESTAMPTZ,
    last_complete_scan_at TIMESTAMPTZ,
    CONSTRAINT upstream_price_monitor_model_scan_singleton CHECK (id = 1)
);
INSERT INTO upstream_price_monitor_model_scan_state (id) VALUES (1) ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS upstream_price_monitor_models (
    model_key VARCHAR(255) PRIMARY KEY,
    model_name VARCHAR(255) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'discovered',
    domestic_candidate BOOLEAN NOT NULL DEFAULT FALSE,
    seen_account_count INTEGER NOT NULL DEFAULT 0,
    expected_account_count INTEGER NOT NULL DEFAULT 0,
    missing_runs INTEGER NOT NULL DEFAULT 0,
    last_complete_revision BIGINT NOT NULL DEFAULT 0,
    first_seen_at TIMESTAMPTZ,
    last_seen_at TIMESTAMPTZ,
    last_missing_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT upstream_price_monitor_models_status_check CHECK (
        status IN ('managed','discovered','suspected_retired','ignored','retired')
    ),
    CONSTRAINT upstream_price_monitor_models_counts_check CHECK (
        seen_account_count >= 0 AND expected_account_count >= 0 AND missing_runs >= 0 AND last_complete_revision >= 0
    )
);
CREATE INDEX IF NOT EXISTS idx_upstream_price_monitor_models_status
    ON upstream_price_monitor_models (status, domestic_candidate, model_key);

INSERT INTO upstream_price_monitor_models (model_key,model_name,status,domestic_candidate,first_seen_at,last_seen_at)
SELECT LOWER(model_name),model_name,'managed',TRUE,NOW(),NOW()
FROM UNNEST(ARRAY[
    'deepseek-v4-flash-0731', 'deepseek-v4-pro-0813', 'deepseek-v4-flash-vision-exp',
    'kimi-k2.6', 'kimi-k2.7-code', 'kimi-k3',
    'glm-5.1', 'glm-5.2', 'glm-5.3', 'glm-5.3-flash',
    'MiniMax-M2.7', 'MiniMax-M2.7-highspeed', 'MiniMax-M3',
    'qwen3.7-max', 'qwen3.8-max', 'mimo-v2.5', 'mimo-v2.5-pro', 'hy3'
]) AS seed(model_name)
ON CONFLICT (model_key) DO NOTHING;

CREATE TABLE IF NOT EXISTS upstream_price_monitor_evidence (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL REFERENCES upstream_price_monitor_runs(id) ON DELETE CASCADE,
    account_id BIGINT NOT NULL,
    model_name VARCHAR(255) NOT NULL,
    billing_mode VARCHAR(24) NOT NULL DEFAULT 'token',
    status VARCHAR(24) NOT NULL,
    source VARCHAR(24) NOT NULL,
    reconciliation_status VARCHAR(32) NOT NULL,
    context_key VARCHAR(128) NOT NULL DEFAULT 'default',
    observed_at TIMESTAMPTZ NOT NULL,
    sample_count INTEGER NOT NULL DEFAULT 0,
    local_delta JSONB NOT NULL DEFAULT '{}'::jsonb,
    remote_delta JSONB NOT NULL DEFAULT '{}'::jsonb,
    prices JSONB NOT NULL DEFAULT '{}'::jsonb,
    current_prices JSONB NOT NULL DEFAULT '{}'::jsonb,
    suggested_prices JSONB NOT NULL DEFAULT '{}'::jsonb,
    display_multiplier_current NUMERIC(12, 6),
    display_multiplier_suggested NUMERIC(12, 6),
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT upstream_price_monitor_evidence_identity UNIQUE (run_id, account_id, model_name, context_key),
    CONSTRAINT upstream_price_monitor_evidence_mode_check CHECK (billing_mode IN ('token', 'per_request')),
    CONSTRAINT upstream_price_monitor_evidence_status_check CHECK (status IN ('trusted', 'pending', 'mismatch', 'stale', 'unobservable')),
    CONSTRAINT upstream_price_monitor_evidence_source_check CHECK (source IN ('user_request', 'active_probe', 'price_page')),
    CONSTRAINT upstream_price_monitor_evidence_reconciliation_check CHECK (reconciliation_status IN ('baseline', 'matched', 'no_activity', 'mismatch', 'remote_reset', 'mixed_context')),
    CONSTRAINT upstream_price_monitor_evidence_samples_check CHECK (sample_count >= 0)
);

CREATE INDEX IF NOT EXISTS idx_upstream_price_monitor_evidence_latest
    ON upstream_price_monitor_evidence (model_name, context_key, observed_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_upstream_price_monitor_evidence_run
    ON upstream_price_monitor_evidence (run_id, id);
CREATE INDEX IF NOT EXISTS idx_upstream_price_monitor_active_cost
    ON upstream_price_monitor_evidence (created_at) WHERE source = 'active_probe';
CREATE INDEX IF NOT EXISTS idx_upstream_price_monitor_observations
    ON upstream_price_monitor_evidence (account_id, LOWER(model_name), context_key, observed_at DESC, id DESC)
    WHERE reconciliation_status = 'matched';
CREATE INDEX IF NOT EXISTS idx_upstream_price_monitor_coverage
    ON upstream_price_monitor_evidence (account_id, LOWER(model_name), observed_at DESC)
    WHERE status = 'trusted' AND billing_mode = 'token';

-- One independent checkpoint per production account and model prevents a
-- delayed or polluted model from consuming another model's clean window.
CREATE TABLE IF NOT EXISTS upstream_price_monitor_usage_checkpoints (
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    model_name VARCHAR(255) NOT NULL,
    account_identity_hash CHAR(64) NOT NULL,
    remote_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    ledger_date DATE NOT NULL,
    billing_context_hash CHAR(64) NOT NULL DEFAULT '',
    local_usage_log_id BIGINT NOT NULL DEFAULT 0,
    captured_at TIMESTAMPTZ NOT NULL,
    active_probe_pending BOOLEAN NOT NULL DEFAULT FALSE,
    active_probe_started_at TIMESTAMPTZ,
    revision BIGINT NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (account_id, model_name),
    CONSTRAINT upstream_price_monitor_checkpoint_watermark_check CHECK (local_usage_log_id >= 0),
    CONSTRAINT upstream_price_monitor_checkpoint_probe_state_check CHECK (
        (active_probe_pending AND active_probe_started_at IS NOT NULL) OR
        (NOT active_probe_pending AND active_probe_started_at IS NULL)
    ),
    CONSTRAINT upstream_price_monitor_checkpoint_revision_check CHECK (revision > 0)
);
