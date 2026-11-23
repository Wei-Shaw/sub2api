-- 253_upstream_price_monitor.sql
-- TeamoRouter 上游价格巡检：单例配置 + 执行记录 + 明细

CREATE TABLE IF NOT EXISTS upstream_price_monitor_configs (
    id BIGINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    interval_seconds INT NOT NULL DEFAULT 43200,
    channel_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    notify_emails JSONB NOT NULL DEFAULT '[]'::jsonb,
    profit_min DOUBLE PRECISION NOT NULL DEFAULT 0.25,
    profit_max DOUBLE PRECISION NOT NULL DEFAULT 0.35,
    target_profit DOUBLE PRECISION,
    last_run_at TIMESTAMPTZ,
    updated_by BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_upstream_price_monitor_interval
        CHECK (interval_seconds >= 3600 AND interval_seconds <= 86400),
    CONSTRAINT chk_upstream_price_monitor_profit
        CHECK (profit_min >= 0 AND profit_max > 0 AND profit_min <= profit_max AND profit_max < 1)
);

INSERT INTO upstream_price_monitor_configs (id)
VALUES (1)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS upstream_price_monitor_runs (
    id BIGSERIAL PRIMARY KEY,
    status VARCHAR(20) NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    upstream_fetched_at TIMESTAMPTZ,
    model_count INT NOT NULL DEFAULT 0,
    matched_count INT NOT NULL DEFAULT 0,
    out_of_range_count INT NOT NULL DEFAULT 0,
    summary TEXT NOT NULL DEFAULT '',
    error_message TEXT,
    channel_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    profit_min DOUBLE PRECISION NOT NULL DEFAULT 0,
    profit_max DOUBLE PRECISION NOT NULL DEFAULT 0,
    target_profit DOUBLE PRECISION,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_upstream_price_monitor_run_status
        CHECK (status IN ('running', 'passed', 'failed', 'error'))
);

CREATE INDEX IF NOT EXISTS idx_upstream_price_monitor_runs_started_at
    ON upstream_price_monitor_runs (started_at DESC);

CREATE TABLE IF NOT EXISTS upstream_price_monitor_run_items (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL REFERENCES upstream_price_monitor_runs(id) ON DELETE CASCADE,
    channel_id BIGINT NOT NULL,
    channel_name VARCHAR(200) NOT NULL DEFAULT '',
    model VARCHAR(200) NOT NULL,
    status VARCHAR(32) NOT NULL,
    upstream_input DOUBLE PRECISION,
    upstream_output DOUBLE PRECISION,
    upstream_cache_read DOUBLE PRECISION,
    upstream_cache_write DOUBLE PRECISION,
    local_input DOUBLE PRECISION,
    local_output DOUBLE PRECISION,
    local_cache_read DOUBLE PRECISION,
    local_cache_write DOUBLE PRECISION,
    suggested_input DOUBLE PRECISION,
    suggested_output DOUBLE PRECISION,
    suggested_cache_read DOUBLE PRECISION,
    suggested_cache_write DOUBLE PRECISION,
    margin_input DOUBLE PRECISION,
    message VARCHAR(500) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_upstream_price_monitor_item_status
        CHECK (status IN ('pass', 'fail', 'unmatched_upstream', 'unmatched_local', 'skipped'))
);

CREATE INDEX IF NOT EXISTS idx_upstream_price_monitor_run_items_run_id
    ON upstream_price_monitor_run_items (run_id);
