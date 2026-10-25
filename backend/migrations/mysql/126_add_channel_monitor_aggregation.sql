-- MySQL 方言版本（翻译自 ../126_add_channel_monitor_aggregation.sql）
-- 翻译说明：
--   - TIMESTAMPTZ → DATETIME(3)，DEFAULT NOW() → DEFAULT CURRENT_TIMESTAMP(3)。
--   - BIGSERIAL → BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY。
--   - 内联 REFERENCES → 显式命名的 CONSTRAINT fk_xxx FOREIGN KEY。
--   - IF NOT EXISTS 修饰去掉（迁移执行器保证单次执行）。
--   - ON CONFLICT (id) DO NOTHING → ON DUPLICATE KEY UPDATE id = id。

-- 1) 给历史明细表加软删除字段
ALTER TABLE channel_monitor_histories
    ADD COLUMN deleted_at DATETIME(3) NULL;

CREATE INDEX idx_channel_monitor_histories_deleted_at
    ON channel_monitor_histories (deleted_at);

-- 2) 创建日聚合表
CREATE TABLE IF NOT EXISTS channel_monitor_daily_rollups (
    id                    BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    monitor_id            BIGINT       NOT NULL,
    model                 VARCHAR(200) NOT NULL,
    bucket_date           DATE         NOT NULL,
    total_checks          INT          NOT NULL DEFAULT 0,
    ok_count              INT          NOT NULL DEFAULT 0,
    operational_count     INT          NOT NULL DEFAULT 0,
    degraded_count        INT          NOT NULL DEFAULT 0,
    failed_count          INT          NOT NULL DEFAULT 0,
    error_count           INT          NOT NULL DEFAULT 0,
    sum_latency_ms        BIGINT       NOT NULL DEFAULT 0,
    count_latency         INT          NOT NULL DEFAULT 0,
    sum_ping_latency_ms   BIGINT       NOT NULL DEFAULT 0,
    count_ping_latency    INT          NOT NULL DEFAULT 0,
    computed_at           DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    deleted_at            DATETIME(3)  NULL,
    CONSTRAINT fk_channel_monitor_daily_rollups_monitor_id
        FOREIGN KEY (monitor_id) REFERENCES channel_monitors(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE UNIQUE INDEX idx_channel_monitor_daily_rollups_unique
    ON channel_monitor_daily_rollups (monitor_id, model, bucket_date);
CREATE INDEX idx_channel_monitor_daily_rollups_bucket
    ON channel_monitor_daily_rollups (bucket_date);
CREATE INDEX idx_channel_monitor_daily_rollups_deleted_at
    ON channel_monitor_daily_rollups (deleted_at);

-- 3) 创建 watermark 表（单行：id=1）
CREATE TABLE IF NOT EXISTS channel_monitor_aggregation_watermark (
    id                   INT          NOT NULL DEFAULT 1 PRIMARY KEY,
    last_aggregated_date DATE         NULL,
    updated_at           DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT channel_monitor_aggregation_watermark_singleton CHECK (id = 1)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO channel_monitor_aggregation_watermark (id, last_aggregated_date, updated_at)
VALUES (1, NULL, NOW(3))
ON DUPLICATE KEY UPDATE id = id;
