-- MySQL 方言版本（翻译自 ../033_ops_monitoring_vnext.sql）
-- Ops Monitoring (vNext): squashed migration (030)
--
-- This repository originally planned Ops vNext as migrations 030-036:
--   030 drop legacy ops tables
--   031 core schema
--   032 pre-aggregation tables
--   033 indexes + optional extensions
--   034 add avg/max to preagg
--   035 add notify_email to alert rules
--   036 seed default alert rules
--
-- Since these migrations have NOT been applied to any environment yet, we squash them
-- into a single 030 migration for easier review and a cleaner migration history.
--
-- Notes:
-- - This is intentionally destructive for ops_* data (error logs / metrics / alerts).
-- - It is idempotent (DROP/CREATE IF EXISTS/IF NOT EXISTS 去除后由迁移执行器保证单次执行),
--   but will wipe ops_* data if re-run.
--
-- 翻译说明：
-- - PG 的 SET LOCAL lock_timeout/statement_timeout 在 MySQL 无对应语法，整句跳过；
-- - client_ip 的 inet 类型 → VARCHAR(45)（可容纳 IPv6 全格式）；
-- - DOUBLE PRECISION → DOUBLE；JSONB → JSON；
-- - 部分（WHERE）非唯一索引 → 去掉 WHERE 的普通索引；
-- - pg_trgm GIN 索引 → 普通 B-tree 索引（TEXT 列使用 191 前缀长度）。

-- =====================================================================
-- 030_ops_drop_legacy_ops_tables.sql
-- =====================================================================

-- Legacy pre-aggregation tables (from 026 and/or previous branches)
DROP TABLE IF EXISTS ops_metrics_daily;
DROP TABLE IF EXISTS ops_metrics_hourly;

-- Core ops tables that may exist in some deployments / branches
DROP TABLE IF EXISTS ops_system_metrics;
DROP TABLE IF EXISTS ops_error_logs;
DROP TABLE IF EXISTS ops_alert_events;
DROP TABLE IF EXISTS ops_alert_rules;
DROP TABLE IF EXISTS ops_job_heartbeats;
DROP TABLE IF EXISTS ops_retry_attempts;

-- Optional legacy tables (best-effort cleanup)
DROP TABLE IF EXISTS ops_scheduled_reports;
DROP TABLE IF EXISTS ops_group_availability_configs;
DROP TABLE IF EXISTS ops_group_availability_events;

-- Optional legacy views/indexes
DROP VIEW IF EXISTS ops_latest_metrics;

-- =====================================================================
-- 031_ops_core_schema.sql
-- =====================================================================

-- Ops Monitoring (vNext): core schema (errors / retries / metrics / jobs / alerts)
--
-- Design goals:
-- - Support global filtering (time/platform/group) across all ops modules.
-- - Persist enough context for two retry modes (client retry / pinned upstream retry).
-- - Make ops background jobs observable via job heartbeats.
-- - Keep schema stable and indexes targeted (high-write tables).
--
-- Notes:
-- - ops_* tables intentionally avoid strict foreign keys to reduce write amplification/locks.

-- ============================================
-- 1) ops_error_logs: error log details (high-write)
-- ============================================

CREATE TABLE IF NOT EXISTS ops_error_logs (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,

    -- Correlation / identities
    request_id VARCHAR(64),
    client_request_id VARCHAR(64),
    user_id BIGINT,
    api_key_id BIGINT,
    account_id BIGINT,
    group_id BIGINT,
    client_ip VARCHAR(45),

    -- Dimensions for global filtering
    platform VARCHAR(32),

    -- Request metadata
    model VARCHAR(100),
    request_path VARCHAR(256),
    stream BOOLEAN NOT NULL DEFAULT FALSE,
    user_agent TEXT,

    -- Core error classification
    error_phase VARCHAR(32) NOT NULL,
    error_type VARCHAR(64) NOT NULL,
    severity VARCHAR(8) NOT NULL DEFAULT 'P2',
    status_code INT,

    -- vNext metric semantics
    is_business_limited BOOLEAN NOT NULL DEFAULT FALSE,

    -- Error details (sanitized/truncated at ingest time)
    error_message TEXT,
    error_body TEXT,

    -- Provider/upstream details (optional; useful for trends & account health)
    error_source VARCHAR(64),
    error_owner VARCHAR(32),
    account_status VARCHAR(50),
    upstream_status_code INT,
    upstream_error_message TEXT,
    upstream_error_detail TEXT,
    provider_error_code VARCHAR(64),
    provider_error_type VARCHAR(64),
    network_error_type VARCHAR(50),
    retry_after_seconds INT,

    -- Timings (ms) - optional
    duration_ms INT,
    time_to_first_token_ms BIGINT,
    auth_latency_ms BIGINT,
    routing_latency_ms BIGINT,
    upstream_latency_ms BIGINT,
    response_latency_ms BIGINT,

    -- Retry context (only stored for error requests)
    request_body JSON,
    request_headers JSON,
    request_body_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    request_body_bytes INT,

    -- Retryability flags (best-effort classification)
    is_retryable BOOLEAN NOT NULL DEFAULT FALSE,
    retry_count INT NOT NULL DEFAULT 0,

    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- (comment skipped: ops_error_logs 表注释)

-- ============================================
-- 2) ops_retry_attempts: audit log for retries
-- ============================================

CREATE TABLE IF NOT EXISTS ops_retry_attempts (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,

    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),

    requested_by_user_id BIGINT,
    source_error_id BIGINT,

    -- client|upstream
    mode VARCHAR(16) NOT NULL,
    pinned_account_id BIGINT,

    -- queued|running|succeeded|failed
    status VARCHAR(16) NOT NULL DEFAULT 'queued',
    started_at DATETIME(3) NULL,
    finished_at DATETIME(3) NULL,
    duration_ms BIGINT,

    -- Optional result correlation
    result_request_id VARCHAR(64),
    result_error_id BIGINT,
    result_usage_request_id VARCHAR(64),

    error_message TEXT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- (comment skipped: ops_retry_attempts 表注释)

-- ============================================
-- 3) ops_system_metrics: system + request window snapshots
-- ============================================

CREATE TABLE IF NOT EXISTS ops_system_metrics (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,

    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    window_minutes INT NOT NULL DEFAULT 1,

    -- Optional dimensions (only if collector chooses to write per-dimension snapshots)
    platform VARCHAR(32),
    group_id BIGINT,

    -- Core counts
    success_count BIGINT NOT NULL DEFAULT 0,
    error_count_total BIGINT NOT NULL DEFAULT 0,
    business_limited_count BIGINT NOT NULL DEFAULT 0,
    error_count_sla BIGINT NOT NULL DEFAULT 0,

    upstream_error_count_excl_429_529 BIGINT NOT NULL DEFAULT 0,
    upstream_429_count BIGINT NOT NULL DEFAULT 0,
    upstream_529_count BIGINT NOT NULL DEFAULT 0,

    token_consumed BIGINT NOT NULL DEFAULT 0,

    -- Rates
    qps DOUBLE,
    tps DOUBLE,

    -- Duration percentiles (ms) - success requests
    duration_p50_ms INT,
    duration_p90_ms INT,
    duration_p95_ms INT,
    duration_p99_ms INT,
    duration_avg_ms DOUBLE,
    duration_max_ms INT,

    -- TTFT percentiles (ms) - success requests (streaming)
    ttft_p50_ms INT,
    ttft_p90_ms INT,
    ttft_p95_ms INT,
    ttft_p99_ms INT,
    ttft_avg_ms DOUBLE,
    ttft_max_ms INT,

    -- System resources
    cpu_usage_percent DOUBLE,
    memory_used_mb BIGINT,
    memory_total_mb BIGINT,
    memory_usage_percent DOUBLE,

    -- Dependency health (best-effort)
    db_ok BOOLEAN,
    redis_ok BOOLEAN,

    -- DB pool & runtime
    db_conn_active INT,
    db_conn_idle INT,
    db_conn_waiting INT,
    goroutine_count INT,

    -- Queue / concurrency
    concurrency_queue_depth INT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- (comment skipped: ops_system_metrics 表注释)

-- ============================================
-- 4) ops_job_heartbeats: background jobs health
-- ============================================

CREATE TABLE IF NOT EXISTS ops_job_heartbeats (
    job_name VARCHAR(64) NOT NULL PRIMARY KEY,

    last_run_at DATETIME(3) NULL,
    last_success_at DATETIME(3) NULL,
    last_error_at DATETIME(3) NULL,
    last_error TEXT,
    last_duration_ms BIGINT,

    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- (comment skipped: ops_job_heartbeats 表注释)

-- ============================================
-- 5) ops_alert_rules / ops_alert_events
-- ============================================

CREATE TABLE IF NOT EXISTS ops_alert_rules (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,

    name VARCHAR(128) NOT NULL,
    description TEXT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,

    severity VARCHAR(16) NOT NULL DEFAULT 'warning',

    -- Metric definition
    metric_type VARCHAR(64) NOT NULL,
    operator VARCHAR(8) NOT NULL,
    threshold DOUBLE NOT NULL,

    window_minutes INT NOT NULL DEFAULT 5,
    sustained_minutes INT NOT NULL DEFAULT 5,
    cooldown_minutes INT NOT NULL DEFAULT 10,

    -- Optional scoping: platform/group filters etc.
    filters JSON,

    last_triggered_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE UNIQUE INDEX idx_ops_alert_rules_name_unique
    ON ops_alert_rules (name);

CREATE INDEX idx_ops_alert_rules_enabled
    ON ops_alert_rules (enabled);

CREATE TABLE IF NOT EXISTS ops_alert_events (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,

    rule_id BIGINT,
    severity VARCHAR(16) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'firing',

    title VARCHAR(200),
    description TEXT,

    metric_value DOUBLE,
    threshold_value DOUBLE,
    dimensions JSON,

    fired_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    resolved_at DATETIME(3) NULL,

    email_sent BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_ops_alert_events_rule_status
    ON ops_alert_events (rule_id, status);

CREATE INDEX idx_ops_alert_events_fired_at
    ON ops_alert_events (fired_at DESC);

-- =====================================================================
-- 032_ops_preaggregation_tables.sql
-- =====================================================================

-- Ops Monitoring (vNext): pre-aggregation tables
--
-- Purpose:
-- - Provide stable query performance for 1–24h windows (and beyond), avoiding expensive
--   percentile_cont scans on raw logs for every dashboard refresh.
-- - Support global filter dimensions: overall / platform / group.
--
-- Design note:
-- - We keep a single table with nullable platform/group_id, and enforce uniqueness via a
--   COALESCE-based functional unique index（MySQL 函数索引写法，每个表达式键部分单独加括号）.

-- ============================================
-- 1) ops_metrics_hourly
-- ============================================

CREATE TABLE IF NOT EXISTS ops_metrics_hourly (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,

    bucket_start DATETIME(3) NOT NULL,
    platform VARCHAR(32),
    group_id BIGINT,

    success_count BIGINT NOT NULL DEFAULT 0,
    error_count_total BIGINT NOT NULL DEFAULT 0,
    business_limited_count BIGINT NOT NULL DEFAULT 0,
    error_count_sla BIGINT NOT NULL DEFAULT 0,

    upstream_error_count_excl_429_529 BIGINT NOT NULL DEFAULT 0,
    upstream_429_count BIGINT NOT NULL DEFAULT 0,
    upstream_529_count BIGINT NOT NULL DEFAULT 0,

    token_consumed BIGINT NOT NULL DEFAULT 0,

    -- Duration percentiles (ms)
    duration_p50_ms INT,
    duration_p90_ms INT,
    duration_p95_ms INT,
    duration_p99_ms INT,

    -- TTFT percentiles (ms)
    ttft_p50_ms INT,
    ttft_p90_ms INT,
    ttft_p95_ms INT,
    ttft_p99_ms INT,

    computed_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Uniqueness across three “dimension modes” (overall / platform / group).
CREATE UNIQUE INDEX idx_ops_metrics_hourly_unique_dim
    ON ops_metrics_hourly (
        bucket_start,
        (COALESCE(platform, '')),
        (COALESCE(group_id, 0))
    );

CREATE INDEX idx_ops_metrics_hourly_bucket
    ON ops_metrics_hourly (bucket_start DESC);

-- （PG 版为部分索引，非唯一 → 去掉 WHERE）
CREATE INDEX idx_ops_metrics_hourly_platform_bucket
    ON ops_metrics_hourly (platform, bucket_start DESC);

CREATE INDEX idx_ops_metrics_hourly_group_bucket
    ON ops_metrics_hourly (group_id, bucket_start DESC);

-- (comment skipped: ops_metrics_hourly 表注释)

-- ============================================
-- 2) ops_metrics_daily (optional; for longer windows)
-- ============================================

CREATE TABLE IF NOT EXISTS ops_metrics_daily (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,

    bucket_date DATE NOT NULL,
    platform VARCHAR(32),
    group_id BIGINT,

    success_count BIGINT NOT NULL DEFAULT 0,
    error_count_total BIGINT NOT NULL DEFAULT 0,
    business_limited_count BIGINT NOT NULL DEFAULT 0,
    error_count_sla BIGINT NOT NULL DEFAULT 0,

    upstream_error_count_excl_429_529 BIGINT NOT NULL DEFAULT 0,
    upstream_429_count BIGINT NOT NULL DEFAULT 0,
    upstream_529_count BIGINT NOT NULL DEFAULT 0,

    token_consumed BIGINT NOT NULL DEFAULT 0,

    duration_p50_ms INT,
    duration_p90_ms INT,
    duration_p95_ms INT,
    duration_p99_ms INT,

    ttft_p50_ms INT,
    ttft_p90_ms INT,
    ttft_p95_ms INT,
    ttft_p99_ms INT,

    computed_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE UNIQUE INDEX idx_ops_metrics_daily_unique_dim
    ON ops_metrics_daily (
        bucket_date,
        (COALESCE(platform, '')),
        (COALESCE(group_id, 0))
    );

CREATE INDEX idx_ops_metrics_daily_bucket
    ON ops_metrics_daily (bucket_date DESC);

-- （PG 版为部分索引，非唯一 → 去掉 WHERE）
CREATE INDEX idx_ops_metrics_daily_platform_bucket
    ON ops_metrics_daily (platform, bucket_date DESC);

CREATE INDEX idx_ops_metrics_daily_group_bucket
    ON ops_metrics_daily (group_id, bucket_date DESC);

-- (comment skipped: ops_metrics_daily 表注释)

-- =====================================================================
-- 033_ops_indexes_and_extensions.sql
-- =====================================================================

-- Ops Monitoring (vNext): indexes
-- （PG 版的 pg_trgm 扩展依赖已移除，模糊搜索索引统一改为普通 B-tree 索引。）

-- ============================================
-- 1) Core btree indexes (always safe)
-- ============================================

-- ops_error_logs
CREATE INDEX idx_ops_error_logs_created_at
    ON ops_error_logs (created_at DESC);

CREATE INDEX idx_ops_error_logs_platform_time
    ON ops_error_logs (platform, created_at DESC);

-- （PG 版为部分索引 WHERE group_id IS NOT NULL → 去掉 WHERE）
CREATE INDEX idx_ops_error_logs_group_time
    ON ops_error_logs (group_id, created_at DESC);

-- （PG 版为部分索引 WHERE account_id IS NOT NULL → 去掉 WHERE）
CREATE INDEX idx_ops_error_logs_account_time
    ON ops_error_logs (account_id, created_at DESC);

CREATE INDEX idx_ops_error_logs_status_time
    ON ops_error_logs (status_code, created_at DESC);

CREATE INDEX idx_ops_error_logs_phase_time
    ON ops_error_logs (error_phase, created_at DESC);

CREATE INDEX idx_ops_error_logs_type_time
    ON ops_error_logs (error_type, created_at DESC);

CREATE INDEX idx_ops_error_logs_request_id
    ON ops_error_logs (request_id);

CREATE INDEX idx_ops_error_logs_client_request_id
    ON ops_error_logs (client_request_id);

-- ops_system_metrics
CREATE INDEX idx_ops_system_metrics_created_at
    ON ops_system_metrics (created_at DESC);

CREATE INDEX idx_ops_system_metrics_window_time
    ON ops_system_metrics (window_minutes, created_at DESC);

-- （PG 版为部分索引 → 去掉 WHERE）
CREATE INDEX idx_ops_system_metrics_platform_time
    ON ops_system_metrics (platform, created_at DESC);

-- （PG 版为部分索引 → 去掉 WHERE）
CREATE INDEX idx_ops_system_metrics_group_time
    ON ops_system_metrics (group_id, created_at DESC);

-- ops_retry_attempts
CREATE INDEX idx_ops_retry_attempts_created_at
    ON ops_retry_attempts (created_at DESC);

-- （PG 版为部分索引 → 去掉 WHERE）
CREATE INDEX idx_ops_retry_attempts_source_error
    ON ops_retry_attempts (source_error_id, created_at DESC);

-- Prevent concurrent retries for the same ops_error_logs row (race-free, multi-instance safe).
-- （PG 版为部分唯一索引 WHERE source_error_id IS NOT NULL AND status IN (...) → 函数键部分）
CREATE UNIQUE INDEX idx_ops_retry_attempts_unique_active
    ON ops_retry_attempts (
        source_error_id,
        (IF(source_error_id IS NOT NULL AND status IN ('queued', 'running'), 0, NULL))
    );

-- ============================================
-- 2) Fuzzy search indexes（原 pg_trgm GIN 索引 → 普通 B-tree 索引）
-- ============================================

-- request_id / client_request_id fuzzy search
CREATE INDEX idx_ops_error_logs_request_id_trgm
    ON ops_error_logs (request_id);
CREATE INDEX idx_ops_error_logs_client_request_id_trgm
    ON ops_error_logs (client_request_id);

-- error_message fuzzy search（TEXT 列需要前缀长度）
CREATE INDEX idx_ops_error_logs_error_message_trgm
    ON ops_error_logs (error_message(191));

-- =====================================================================
-- 034_ops_preaggregation_add_avg_max.sql
-- =====================================================================

-- Ops Monitoring (vNext): extend pre-aggregation tables with avg/max latency fields
--
-- Why:
-- - The dashboard overview returns avg/max for duration/TTFT.
-- - Hourly/daily pre-aggregation tables originally stored only p50/p90/p95/p99, which makes
--   it impossible to answer avg/max in preagg mode without falling back to raw scans.
--
-- NOTE: We keep the existing p50/p90/p95/p99 columns as-is; these are still used for
--       approximate long-window summaries.

-- Hourly table
ALTER TABLE ops_metrics_hourly
    ADD COLUMN duration_avg_ms DOUBLE NULL,
    ADD COLUMN duration_max_ms INT NULL,
    ADD COLUMN ttft_avg_ms DOUBLE NULL,
    ADD COLUMN ttft_max_ms INT NULL;

-- Daily table
ALTER TABLE ops_metrics_daily
    ADD COLUMN duration_avg_ms DOUBLE NULL,
    ADD COLUMN duration_max_ms INT NULL,
    ADD COLUMN ttft_avg_ms DOUBLE NULL,
    ADD COLUMN ttft_max_ms INT NULL;

-- =====================================================================
-- 035_ops_alert_rules_notify_email.sql
-- =====================================================================

-- Ops Monitoring (vNext): alert rule notify settings

ALTER TABLE ops_alert_rules
    ADD COLUMN notify_email BOOLEAN NOT NULL DEFAULT TRUE;

-- =====================================================================
-- 036_ops_seed_default_alert_rules.sql
-- =====================================================================

-- Ops Monitoring (vNext): seed default alert rules
--
-- Goal:
-- - Provide "out of the box" alert rules so the Ops dashboard can immediately show alert events.
-- - Keep inserts idempotent via ON DUPLICATE KEY UPDATE（PG 的 ON CONFLICT (name) DO NOTHING）。
--
-- Notes:
-- - Thresholds are intentionally conservative defaults and should be tuned per deployment.
-- - Metric semantics follow vNext:
--   - success_rate / error_rate are based on SLA-scope counts (exclude is_business_limited).
--   - upstream_error_rate excludes 429/529.

-- 1) High error rate (P1)
INSERT INTO ops_alert_rules (
    name, description, enabled, metric_type, operator, threshold,
    window_minutes, sustained_minutes, severity, notify_email, cooldown_minutes,
    created_at, updated_at
) VALUES (
    '错误率过高',
    '当错误率超过 5% 且持续 5 分钟时触发告警',
    TRUE, 'error_rate', '>', 5.0, 5, 5, 'P1', TRUE, 20, NOW(3), NOW(3)
) ON DUPLICATE KEY UPDATE id = id;

-- 2) Low success rate (P0)
INSERT INTO ops_alert_rules (
    name, description, enabled, metric_type, operator, threshold,
    window_minutes, sustained_minutes, severity, notify_email, cooldown_minutes,
    created_at, updated_at
) VALUES (
    '成功率过低',
    '当成功率低于 95% 且持续 5 分钟时触发告警（服务可用性下降）',
    TRUE, 'success_rate', '<', 95.0, 5, 5, 'P0', TRUE, 15, NOW(3), NOW(3)
) ON DUPLICATE KEY UPDATE id = id;

-- 3) P99 latency too high (P2)
INSERT INTO ops_alert_rules (
    name, description, enabled, metric_type, operator, threshold,
    window_minutes, sustained_minutes, severity, notify_email, cooldown_minutes,
    created_at, updated_at
) VALUES (
    'P99延迟过高',
    '当 P99 延迟超过 3000ms 且持续 10 分钟时触发告警',
    TRUE, 'p99_latency_ms', '>', 3000.0, 5, 10, 'P2', TRUE, 30, NOW(3), NOW(3)
) ON DUPLICATE KEY UPDATE id = id;

-- 4) P95 latency too high (P2)
INSERT INTO ops_alert_rules (
    name, description, enabled, metric_type, operator, threshold,
    window_minutes, sustained_minutes, severity, notify_email, cooldown_minutes,
    created_at, updated_at
) VALUES (
    'P95延迟过高',
    '当 P95 延迟超过 2000ms 且持续 10 分钟时触发告警',
    TRUE, 'p95_latency_ms', '>', 2000.0, 5, 10, 'P2', TRUE, 30, NOW(3), NOW(3)
) ON DUPLICATE KEY UPDATE id = id;

-- 5) CPU usage too high (P2)
INSERT INTO ops_alert_rules (
    name, description, enabled, metric_type, operator, threshold,
    window_minutes, sustained_minutes, severity, notify_email, cooldown_minutes,
    created_at, updated_at
) VALUES (
    'CPU使用率过高',
    '当 CPU 使用率超过 85% 且持续 10 分钟时触发告警',
    TRUE, 'cpu_usage_percent', '>', 85.0, 5, 10, 'P2', TRUE, 30, NOW(3), NOW(3)
) ON DUPLICATE KEY UPDATE id = id;

-- 6) Memory usage too high (P1)
INSERT INTO ops_alert_rules (
    name, description, enabled, metric_type, operator, threshold,
    window_minutes, sustained_minutes, severity, notify_email, cooldown_minutes,
    created_at, updated_at
) VALUES (
    '内存使用率过高',
    '当内存使用率超过 90% 且持续 10 分钟时触发告警（可能导致 OOM）',
    TRUE, 'memory_usage_percent', '>', 90.0, 5, 10, 'P1', TRUE, 20, NOW(3), NOW(3)
) ON DUPLICATE KEY UPDATE id = id;

-- 7) Concurrency queue buildup (P1)
INSERT INTO ops_alert_rules (
    name, description, enabled, metric_type, operator, threshold,
    window_minutes, sustained_minutes, severity, notify_email, cooldown_minutes,
    created_at, updated_at
) VALUES (
    '并发队列积压',
    '当并发队列深度超过 100 且持续 5 分钟时触发告警（系统处理能力不足）',
    TRUE, 'concurrency_queue_depth', '>', 100.0, 5, 5, 'P1', TRUE, 20, NOW(3), NOW(3)
) ON DUPLICATE KEY UPDATE id = id;

-- 8) Extremely high error rate (P0)
INSERT INTO ops_alert_rules (
    name, description, enabled, metric_type, operator, threshold,
    window_minutes, sustained_minutes, severity, notify_email, cooldown_minutes,
    created_at, updated_at
) VALUES (
    '错误率极高',
    '当错误率超过 20% 且持续 1 分钟时触发告警（服务严重异常）',
    TRUE, 'error_rate', '>', 20.0, 1, 1, 'P0', TRUE, 15, NOW(3), NOW(3)
) ON DUPLICATE KEY UPDATE id = id;

-- Ops Monitoring vNext: add Redis pool stats fields to system metrics snapshots.

ALTER TABLE ops_system_metrics
  ADD COLUMN redis_conn_total INT NULL,
  ADD COLUMN redis_conn_idle INT NULL;

-- (comment skipped: ops_system_metrics.redis_conn_total)
-- (comment skipped: ops_system_metrics.redis_conn_idle)
