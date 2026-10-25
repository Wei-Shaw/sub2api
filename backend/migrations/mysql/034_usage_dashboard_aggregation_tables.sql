-- MySQL 方言版本（翻译自 ../034_usage_dashboard_aggregation_tables.sql）

-- Usage dashboard aggregation tables (hourly/daily) + active-user dedup + watermark.
-- These tables support Admin Dashboard statistics without full-table scans on usage_logs.

-- Hourly aggregates (UTC buckets).
CREATE TABLE IF NOT EXISTS usage_dashboard_hourly (
    bucket_start DATETIME(3) NOT NULL PRIMARY KEY,
    total_requests BIGINT NOT NULL DEFAULT 0,
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    cache_creation_tokens BIGINT NOT NULL DEFAULT 0,
    cache_read_tokens BIGINT NOT NULL DEFAULT 0,
    total_cost DECIMAL(20, 10) NOT NULL DEFAULT 0,
    actual_cost DECIMAL(20, 10) NOT NULL DEFAULT 0,
    total_duration_ms BIGINT NOT NULL DEFAULT 0,
    active_users BIGINT NOT NULL DEFAULT 0,
    computed_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_usage_dashboard_hourly_bucket_start
    ON usage_dashboard_hourly (bucket_start DESC);

-- (comment skipped)
-- (comment skipped)
-- (comment skipped)

-- Daily aggregates (UTC dates).
CREATE TABLE IF NOT EXISTS usage_dashboard_daily (
    bucket_date DATE NOT NULL PRIMARY KEY,
    total_requests BIGINT NOT NULL DEFAULT 0,
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    cache_creation_tokens BIGINT NOT NULL DEFAULT 0,
    cache_read_tokens BIGINT NOT NULL DEFAULT 0,
    total_cost DECIMAL(20, 10) NOT NULL DEFAULT 0,
    actual_cost DECIMAL(20, 10) NOT NULL DEFAULT 0,
    total_duration_ms BIGINT NOT NULL DEFAULT 0,
    active_users BIGINT NOT NULL DEFAULT 0,
    computed_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_usage_dashboard_daily_bucket_date
    ON usage_dashboard_daily (bucket_date DESC);

-- (comment skipped)
-- (comment skipped)
-- (comment skipped)

-- Hourly active user dedup table.
CREATE TABLE IF NOT EXISTS usage_dashboard_hourly_users (
    bucket_start DATETIME(3) NOT NULL,
    user_id BIGINT NOT NULL,
    PRIMARY KEY (bucket_start, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_usage_dashboard_hourly_users_bucket_start
    ON usage_dashboard_hourly_users (bucket_start);

-- Daily active user dedup table.
CREATE TABLE IF NOT EXISTS usage_dashboard_daily_users (
    bucket_date DATE NOT NULL,
    user_id BIGINT NOT NULL,
    PRIMARY KEY (bucket_date, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_usage_dashboard_daily_users_bucket_date
    ON usage_dashboard_daily_users (bucket_date);

-- Aggregation watermark table (single row).
-- PG `DEFAULT TIMESTAMPTZ '1970-01-01 00:00:00+00'` → DATETIME 常量默认值
CREATE TABLE IF NOT EXISTS usage_dashboard_aggregation_watermark (
    id INT NOT NULL PRIMARY KEY,
    last_aggregated_at DATETIME(3) NOT NULL DEFAULT '1970-01-01 00:00:00.000',
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO usage_dashboard_aggregation_watermark (id)
VALUES (1)
ON DUPLICATE KEY UPDATE id = id;
