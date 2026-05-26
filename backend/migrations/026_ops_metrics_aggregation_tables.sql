-- Ops monitoring: pre-aggregation tables for dashboard queries
--
-- Problem:
-- The ops dashboard currently runs percentile_cont + GROUP BY queries over large raw tables
-- (usage_logs, ops_error_logs). These will get slower as data grows.
--
-- This migration adds schema-only aggregation tables that can be populated by a future background job.
-- No triggers/functions/jobs are created here (schema only).

-- ============================================
-- Hourly aggregates (per provider/platform)
-- ============================================

CREATE TABLE IF NOT EXISTS ops_metrics_hourly (
    -- Start of the hour bucket (recommended: UTC).
    bucket_start TIMESTAMPTZ NOT NULL,

    -- Provider/platform label (e.g. anthropic/openai/gemini). Mirrors ops_* queries that GROUP BY platform.
    platform VARCHAR(50) NOT NULL,

    -- Traffic counts (use these to compute rates reliably across ranges).
    request_count BIGINT NOT NULL DEFAULT 0,
    success_count BIGINT NOT NULL DEFAULT 0,
    error_count BIGINT NOT NULL DEFAULT 0,

    -- Error breakdown used by provider health UI.
    error_4xx_count BIGINT NOT NULL DEFAULT 0,
    error_5xx_count BIGINT NOT NULL DEFAULT 0,
    timeout_count BIGINT NOT NULL DEFAULT 0,

    -- Latency aggregates (ms).
    avg_latency_ms DOUBLE PRECISION,
    p99_latency_ms DOUBLE PRECISION,

    -- Convenience rate (percentage, 0-100). Still keep counts as source of truth.
    error_rate DOUBLE PRECISION NOT NULL DEFAULT 0,

    -- When this row was last (re)computed by the background job.
    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (bucket_start, platform)
);

CREATE INDEX IF NOT EXISTS idx_ops_metrics_hourly_platform_bucket_start
    ON ops_metrics_hourly (platform, bucket_start DESC);

COMMENT ON TABLE ops_metrics_hourly IS 'Pre-aggregated hourly ops metrics by provider/platform to speed up dashboard queries.';
COMMENT ON COLUMN ops_metrics_hourly.bucket_start IS 'Start timestamp of the hour bucket (recommended UTC).';
COMMENT ON COLUMN ops_metrics_hourly.platform IS 'Provider/platform label (anthropic/openai/gemini, etc).';
COMMENT ON COLUMN ops_metrics_hourly.error_rate IS 'Error rate percentage for the bucket (0-100). Counts remain the source of truth.';
COMMENT ON COLUMN ops_metrics_hourly.computed_at IS 'When the row was last computed/refreshed.';

-- ============================================
-- Daily aggregates (per provider/platform)
-- ============================================

CREATE TABLE IF NOT EXISTS ops_metrics_daily (
    -- Day bucket (recommended: UTC date).
    bucket_date DATE NOT NULL,
    platform VARCHAR(50) NOT NULL,

    request_count BIGINT NOT NULL DEFAULT 0,
    success_count BIGINT NOT NULL DEFAULT 0,
    error_count BIGINT NOT NULL DEFAULT 0,

    error_4xx_count BIGINT NOT NULL DEFAULT 0,
    error_5xx_count BIGINT NOT NULL DEFAULT 0,
    timeout_count BIGINT NOT NULL DEFAULT 0,

    avg_latency_ms DOUBLE PRECISION,
    p99_latency_ms DOUBLE PRECISION,

    error_rate DOUBLE PRECISION NOT NULL DEFAULT 0,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (bucket_date, platform)
);

CREATE INDEX IF NOT EXISTS idx_ops_metrics_daily_platform_bucket_date
    ON ops_metrics_daily (platform, bucket_date DESC);

COMMENT ON TABLE ops_metrics_daily IS 'Pre-aggregated daily ops metrics by provider/platform for longer-term trends.';
COMMENT ON COLUMN ops_metrics_daily.bucket_date IS 'UTC date of the day bucket (recommended).';

-- ============================================
-- Population strategy (future background job)
-- ============================================
--
-- Suggested approach:
-- 1) Compute hourly buckets from raw logs using UTC time-bucketing, then UPSERT into ops_metrics_hourly.
-- 2) Compute daily buckets either directly from raw logs or by rolling up ops_metrics_hourly.
--
-- Notes:
-- - Ensure the job uses a consistent timezone (recommended: SET TIME ZONE ''UTC'') to avoid bucket drift.
-- - Derive the provider/platform similarly to existing dashboard queries:
--     usage_logs: COALESCE(NULLIF(groups.platform, ''), accounts.platform, '')
--     ops_error_logs: COALESCE(NULLIF(ops_error_logs.platform, ''), groups.platform, accounts.platform, '')
-- - Keep request_count/success_count/error_count as the authoritative values; compute error_rate from counts.
--
-- Example (hourly) shape (pseudo-SQL):
--   INSERT INTO ops_metrics_hourly (...)
--   SELECT date_trunc('hour', created_at) AS bucket_start, platform, ...
--   FROM (/* aggregate usage_logs + ops_error_logs */) s
--   ON CONFLICT (bucket_start, platform) DO UPDATE SET ...;
