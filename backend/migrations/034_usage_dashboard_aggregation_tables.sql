-- Usage dashboard aggregation tables (hourly/daily) + active-user dedup + watermark.
-- These tables support Admin Dashboard statistics without full-table scans on usage_logs.

-- Hourly aggregates (UTC buckets).
CREATE TABLE IF NOT EXISTS usage_dashboard_hourly (
    bucket_start TIMESTAMPTZ PRIMARY KEY,
    total_requests BIGINT NOT NULL DEFAULT 0,
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    cache_creation_tokens BIGINT NOT NULL DEFAULT 0,
    cache_read_tokens BIGINT NOT NULL DEFAULT 0,
    total_cost DECIMAL(20, 10) NOT NULL DEFAULT 0,
    actual_cost DECIMAL(20, 10) NOT NULL DEFAULT 0,
    total_duration_ms BIGINT NOT NULL DEFAULT 0,
    active_users BIGINT NOT NULL DEFAULT 0,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_usage_dashboard_hourly_bucket_start
    ON usage_dashboard_hourly (bucket_start DESC);

COMMENT ON TABLE usage_dashboard_hourly IS 'Pre-aggregated hourly usage metrics for admin dashboard (UTC buckets).';
COMMENT ON COLUMN usage_dashboard_hourly.bucket_start IS 'UTC start timestamp of the hour bucket.';
COMMENT ON COLUMN usage_dashboard_hourly.computed_at IS 'When the hourly row was last computed/refreshed.';

-- Daily aggregates (UTC dates).
CREATE TABLE IF NOT EXISTS usage_dashboard_daily (
    bucket_date DATE PRIMARY KEY,
    total_requests BIGINT NOT NULL DEFAULT 0,
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    cache_creation_tokens BIGINT NOT NULL DEFAULT 0,
    cache_read_tokens BIGINT NOT NULL DEFAULT 0,
    total_cost DECIMAL(20, 10) NOT NULL DEFAULT 0,
    actual_cost DECIMAL(20, 10) NOT NULL DEFAULT 0,
    total_duration_ms BIGINT NOT NULL DEFAULT 0,
    active_users BIGINT NOT NULL DEFAULT 0,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_usage_dashboard_daily_bucket_date
    ON usage_dashboard_daily (bucket_date DESC);

COMMENT ON TABLE usage_dashboard_daily IS 'Pre-aggregated daily usage metrics for admin dashboard (UTC dates).';
COMMENT ON COLUMN usage_dashboard_daily.bucket_date IS 'UTC date of the day bucket.';
COMMENT ON COLUMN usage_dashboard_daily.computed_at IS 'When the daily row was last computed/refreshed.';

-- Hourly active user dedup table.
CREATE TABLE IF NOT EXISTS usage_dashboard_hourly_users (
    bucket_start TIMESTAMPTZ NOT NULL,
    user_id BIGINT NOT NULL,
    PRIMARY KEY (bucket_start, user_id)
);

CREATE INDEX IF NOT EXISTS idx_usage_dashboard_hourly_users_bucket_start
    ON usage_dashboard_hourly_users (bucket_start);

-- Daily active user dedup table.
CREATE TABLE IF NOT EXISTS usage_dashboard_daily_users (
    bucket_date DATE NOT NULL,
    user_id BIGINT NOT NULL,
    PRIMARY KEY (bucket_date, user_id)
);

CREATE INDEX IF NOT EXISTS idx_usage_dashboard_daily_users_bucket_date
    ON usage_dashboard_daily_users (bucket_date);

-- Aggregation watermark table (single row).
CREATE TABLE IF NOT EXISTS usage_dashboard_aggregation_watermark (
    id INT PRIMARY KEY,
    last_aggregated_at TIMESTAMPTZ NOT NULL DEFAULT TIMESTAMPTZ '1970-01-01 00:00:00+00',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO usage_dashboard_aggregation_watermark (id)
VALUES (1)
ON CONFLICT (id) DO NOTHING;
