-- MySQL 方言版本（翻译自 ../194_channel_monitor_v2.sql）
-- Passive channel monitor V2. This migration creates empty rollup tables only;
-- backfill is intentionally handled by a bounded background worker.
--
-- 说明：
-- - MySQL 主键不能直接包含 TEXT 列（必须给前缀且总键长受限），故将各 rollup 表
--   主键中的 platform / model / error_category / metric 列由 TEXT 收敛为 VARCHAR(200)/VARCHAR(20)；
-- - BIGINT[]（group_ids）→ JSON，默认空数组 DEFAULT ('[]')（数组语义）；
-- - JSONB → JSON（表达式默认值 DEFAULT ('...')，8.0.13+）；
-- - ON CONFLICT (id) DO NOTHING → ON DUPLICATE KEY UPDATE id = id；
-- - 主键本身已覆盖查询维度，PG 版的二级 DESC 索引原样保留。

CREATE TABLE IF NOT EXISTS channel_monitor_v2_config (
    id SMALLINT NOT NULL DEFAULT 1 PRIMARY KEY CHECK (id = 1),
    version INT NOT NULL DEFAULT 1,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    refresh_interval_seconds INT NOT NULL DEFAULT 60
        CHECK (refresh_interval_seconds IN (60, 300)),
    platforms JSON NOT NULL DEFAULT ('[{"platform":"anthropic","enabled":true,"models":[]},{"platform":"openai","enabled":true,"models":[]},{"platform":"grok","enabled":true,"models":[]},{"platform":"kiro","enabled":true,"models":[]},{"platform":"gemini","enabled":true,"models":[]},{"platform":"antigravity","enabled":true,"models":[]}]'),
    group_ids JSON NOT NULL DEFAULT ('[]'),
    updated_by BIGINT,
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO channel_monitor_v2_config (id) VALUES (1)
ON DUPLICATE KEY UPDATE id = id;

CREATE TABLE IF NOT EXISTS channel_monitor_v2_metrics_1m (
    bucket_start DATETIME(3) NOT NULL,
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
    PRIMARY KEY (bucket_start, platform, group_id, model)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_channel_monitor_v2_metrics_platform_time
    ON channel_monitor_v2_metrics_1m (platform, bucket_start DESC);
CREATE INDEX idx_channel_monitor_v2_metrics_group_time
    ON channel_monitor_v2_metrics_1m (group_id, bucket_start DESC);
CREATE INDEX idx_channel_monitor_v2_metrics_model_time
    ON channel_monitor_v2_metrics_1m (model, bucket_start DESC);

CREATE TABLE IF NOT EXISTS channel_monitor_v2_user_metrics_1m (
    bucket_start DATETIME(3) NOT NULL,
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
    PRIMARY KEY (bucket_start, platform, group_id, model, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_channel_monitor_v2_user_metrics_user_time
    ON channel_monitor_v2_user_metrics_1m (user_id, bucket_start DESC);
CREATE INDEX idx_channel_monitor_v2_user_metrics_time
    ON channel_monitor_v2_user_metrics_1m (bucket_start DESC);

CREATE TABLE IF NOT EXISTS channel_monitor_v2_error_metrics_1m (
    bucket_start DATETIME(3) NOT NULL,
    platform VARCHAR(200) NOT NULL,
    group_id BIGINT NOT NULL DEFAULT 0,
    model VARCHAR(200) NOT NULL,
    error_category VARCHAR(200) NOT NULL,
    taxonomy_version SMALLINT NOT NULL,
    error_requests BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (bucket_start, platform, group_id, model, error_category, taxonomy_version)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_channel_monitor_v2_errors_time
    ON channel_monitor_v2_error_metrics_1m (bucket_start DESC);
CREATE INDEX idx_channel_monitor_v2_errors_category_time
    ON channel_monitor_v2_error_metrics_1m (error_category, bucket_start DESC);

CREATE TABLE IF NOT EXISTS channel_monitor_v2_latency_histograms_1m (
    bucket_start DATETIME(3) NOT NULL,
    platform VARCHAR(200) NOT NULL,
    group_id BIGINT NOT NULL DEFAULT 0,
    model VARCHAR(200) NOT NULL,
    user_id BIGINT NOT NULL DEFAULT 0,
    metric VARCHAR(20) NOT NULL,
    upper_bound_ms INT NOT NULL,
    sample_count BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (bucket_start, platform, group_id, model, user_id, metric, upper_bound_ms),
    CONSTRAINT channel_monitor_v2_latency_histograms_metric_check CHECK (metric IN ('ttft', 'duration'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_channel_monitor_v2_histograms_time
    ON channel_monitor_v2_latency_histograms_1m (bucket_start DESC, metric);

CREATE TABLE IF NOT EXISTS channel_monitor_v2_watermarks (
    id SMALLINT NOT NULL DEFAULT 1 PRIMARY KEY CHECK (id = 1),
    usage_coverage_start DATETIME(3) NULL,
    error_coverage_start DATETIME(3) NULL,
    data_through DATETIME(3) NULL,
    last_successful_at DATETIME(3) NULL,
    backfill_cursor DATETIME(3) NULL,
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO channel_monitor_v2_watermarks (id) VALUES (1)
ON DUPLICATE KEY UPDATE id = id;

-- (comment skipped: channel_monitor_v2_metrics_1m 表注释)
-- (comment skipped: channel_monitor_v2_error_metrics_1m.taxonomy_version 列注释)
