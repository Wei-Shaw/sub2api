-- MySQL 方言版本（翻译自 ../026_ops_metrics_aggregation_tables.sql）
-- Ops monitoring: pre-aggregation tables for dashboard queries
--
-- Problem:
-- The ops dashboard currently runs percentile_cont + GROUP BY queries over large raw tables
-- (usage_logs, ops_error_logs). These will get slower as data grows.
--
-- This migration adds schema-only aggregation tables that can be populated by a future background job.
-- No triggers/functions/jobs are created here (schema only).
--
-- 翻译说明：TIMESTAMPTZ → DATETIME(3)；DOUBLE PRECISION → DOUBLE；
--           DEFAULT NOW() → DEFAULT CURRENT_TIMESTAMP(3)；
--           COMMENT ON TABLE/COLUMN → 整句跳过；
--           (platform, bucket_start DESC) 复合降序索引 MySQL 8 原生支持。

-- ============================================
-- Hourly aggregates (per provider/platform)
-- ============================================

CREATE TABLE IF NOT EXISTS ops_metrics_hourly (
    -- Start of the hour bucket (recommended: UTC).
    bucket_start DATETIME(3) NOT NULL,

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
    avg_latency_ms DOUBLE NULL,
    p99_latency_ms DOUBLE NULL,

    -- Convenience rate (percentage, 0-100). Still keep counts as source of truth.
    error_rate DOUBLE NOT NULL DEFAULT 0,

    -- When this row was last (re)computed by the background job.
    computed_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),

    PRIMARY KEY (bucket_start, platform)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_ops_metrics_hourly_platform_bucket_start
    ON ops_metrics_hourly (platform, bucket_start DESC);

-- (comment skipped) COMMENT ON TABLE ops_metrics_hourly IS 'Pre-aggregated hourly ops metrics by provider/platform to speed up dashboard queries.'
-- (comment skipped) COMMENT ON COLUMN ops_metrics_hourly.bucket_start IS 'Start timestamp of the hour bucket (recommended UTC).'
-- (comment skipped) COMMENT ON COLUMN ops_metrics_hourly.platform IS 'Provider/platform label (anthropic/openai/gemini, etc).'
-- (comment skipped) COMMENT ON COLUMN ops_metrics_hourly.error_rate IS 'Error rate percentage for the bucket (0-100). Counts remain the source of truth.'
-- (comment skipped) COMMENT ON COLUMN ops_metrics_hourly.computed_at IS 'When the row was last computed/refreshed.'

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

    avg_latency_ms DOUBLE NULL,
    p99_latency_ms DOUBLE NULL,

    error_rate DOUBLE NOT NULL DEFAULT 0,
    computed_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),

    PRIMARY KEY (bucket_date, platform)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_ops_metrics_daily_platform_bucket_date
    ON ops_metrics_daily (platform, bucket_date DESC);

-- (comment skipped) COMMENT ON TABLE ops_metrics_daily IS 'Pre-aggregated daily ops metrics by provider/platform for longer-term trends.'
-- (comment skipped) COMMENT ON COLUMN ops_metrics_daily.bucket_date IS 'UTC date of the day bucket (recommended).'

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
