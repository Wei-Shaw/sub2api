-- MySQL 方言版本（翻译自 ../199_channel_monitor_v2_fixed_rollups.sql）
-- Fixed-size Channel Monitor V2 rollups for the 24h / 7d / 30d views.
-- Source of truth remains the 1-minute passive rollup; these tables cache the
-- UI bucket sizes so historical buckets do not need to be re-aggregated on each
-- filter change. The background aggregator overwrites affected buckets.
--
-- 说明：
--   - TIMESTAMPTZ → DATETIME(3)；INTEGER → INT；SMALLINT 保留。
--   - 主键中的 platform / model / error_category / metric 列由 TEXT 收敛为
--     VARCHAR(200)/VARCHAR(20)（MySQL TEXT 不能直接作键列，与 194 迁移口径一致）。
--   - PG 匿名 CHECK 约束按 PG 自动命名规则补名（<table>_<col>_check），
--     CHECK 约束需 MySQL 8.0.16+ 才强制执行。

CREATE TABLE IF NOT EXISTS channel_monitor_v2_metrics_rollup (
    bucket_start DATETIME(3) NOT NULL,
    bucket_seconds INT NOT NULL,
    platform VARCHAR(200) NOT NULL,
    group_id BIGINT NOT NULL DEFAULT 0,
    model VARCHAR(200) NOT NULL,
    success_requests BIGINT NOT NULL DEFAULT 0,
    error_requests BIGINT NOT NULL DEFAULT 0,
    upstream_affected_requests BIGINT NOT NULL DEFAULT 0,
    upstream_attempt_count BIGINT NOT NULL DEFAULT 0,
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    cache_creation_tokens BIGINT NOT NULL DEFAULT 0,
    cache_read_tokens BIGINT NOT NULL DEFAULT 0,
    ttft_sum_ms BIGINT NOT NULL DEFAULT 0,
    ttft_count BIGINT NOT NULL DEFAULT 0,
    duration_sum_ms BIGINT NOT NULL DEFAULT 0,
    duration_count BIGINT NOT NULL DEFAULT 0,
    computed_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (bucket_seconds, bucket_start, platform, group_id, model),
    CONSTRAINT channel_monitor_v2_metrics_rollup_bucket_seconds_check
        CHECK (bucket_seconds IN (300, 3600, 43200, 86400))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_channel_monitor_v2_metrics_rollup_platform_time
    ON channel_monitor_v2_metrics_rollup (bucket_seconds, platform, bucket_start DESC);
CREATE INDEX idx_channel_monitor_v2_metrics_rollup_group_time
    ON channel_monitor_v2_metrics_rollup (bucket_seconds, group_id, bucket_start DESC);
CREATE INDEX idx_channel_monitor_v2_metrics_rollup_model_time
    ON channel_monitor_v2_metrics_rollup (bucket_seconds, model, bucket_start DESC);

CREATE TABLE IF NOT EXISTS channel_monitor_v2_user_metrics_rollup (
    bucket_start DATETIME(3) NOT NULL,
    bucket_seconds INT NOT NULL,
    platform VARCHAR(200) NOT NULL,
    group_id BIGINT NOT NULL DEFAULT 0,
    model VARCHAR(200) NOT NULL,
    user_id BIGINT NOT NULL,
    success_requests BIGINT NOT NULL DEFAULT 0,
    error_requests BIGINT NOT NULL DEFAULT 0,
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    cache_creation_tokens BIGINT NOT NULL DEFAULT 0,
    cache_read_tokens BIGINT NOT NULL DEFAULT 0,
    ttft_sum_ms BIGINT NOT NULL DEFAULT 0,
    ttft_count BIGINT NOT NULL DEFAULT 0,
    duration_sum_ms BIGINT NOT NULL DEFAULT 0,
    duration_count BIGINT NOT NULL DEFAULT 0,
    computed_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (bucket_seconds, bucket_start, platform, group_id, model, user_id),
    CONSTRAINT channel_monitor_v2_user_metrics_rollup_bucket_seconds_check
        CHECK (bucket_seconds IN (300, 3600, 43200, 86400))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_channel_monitor_v2_user_rollup_user_time
    ON channel_monitor_v2_user_metrics_rollup (bucket_seconds, user_id, bucket_start DESC);
CREATE INDEX idx_channel_monitor_v2_user_rollup_time
    ON channel_monitor_v2_user_metrics_rollup (bucket_seconds, bucket_start DESC);

CREATE TABLE IF NOT EXISTS channel_monitor_v2_error_metrics_rollup (
    bucket_start DATETIME(3) NOT NULL,
    bucket_seconds INT NOT NULL,
    platform VARCHAR(200) NOT NULL,
    group_id BIGINT NOT NULL DEFAULT 0,
    model VARCHAR(200) NOT NULL,
    error_category VARCHAR(200) NOT NULL,
    taxonomy_version SMALLINT NOT NULL,
    error_requests BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (bucket_seconds, bucket_start, platform, group_id, model, error_category, taxonomy_version),
    CONSTRAINT channel_monitor_v2_error_metrics_rollup_bucket_seconds_check
        CHECK (bucket_seconds IN (300, 3600, 43200, 86400))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_channel_monitor_v2_errors_rollup_time
    ON channel_monitor_v2_error_metrics_rollup (bucket_seconds, bucket_start DESC);
CREATE INDEX idx_channel_monitor_v2_errors_rollup_category_time
    ON channel_monitor_v2_error_metrics_rollup (bucket_seconds, error_category, bucket_start DESC);

CREATE TABLE IF NOT EXISTS channel_monitor_v2_latency_histograms_rollup (
    bucket_start DATETIME(3) NOT NULL,
    bucket_seconds INT NOT NULL,
    platform VARCHAR(200) NOT NULL,
    group_id BIGINT NOT NULL DEFAULT 0,
    model VARCHAR(200) NOT NULL,
    user_id BIGINT NOT NULL DEFAULT 0,
    metric VARCHAR(20) NOT NULL,
    upper_bound_ms INT NOT NULL,
    sample_count BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (bucket_seconds, bucket_start, platform, group_id, model, user_id, metric, upper_bound_ms),
    CONSTRAINT channel_monitor_v2_latency_histograms_rollup_bucket_chk
        CHECK (bucket_seconds IN (300, 3600, 43200, 86400)),
    CONSTRAINT channel_monitor_v2_latency_histograms_rollup_metric_check
        CHECK (metric IN ('ttft', 'duration'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_channel_monitor_v2_histograms_rollup_time
    ON channel_monitor_v2_latency_histograms_rollup (bucket_seconds, bucket_start DESC, metric);

-- (comment skipped: channel_monitor_v2_metrics_rollup 表注释)
