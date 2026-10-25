-- MySQL 方言版本（翻译自 ../081_create_channels.sql）
-- Create channels table for managing pricing channels.
-- A channel groups multiple groups together and provides custom model pricing.
--
-- 翻译说明：
-- - PG 的 SET LOCAL lock_timeout / statement_timeout 无 MySQL 等价语句，跳过；
-- - 内联 REFERENCES → 显式命名 CONSTRAINT fk_xxx FOREIGN KEY；
-- - TEXT DEFAULT '' / JSONB DEFAULT '[]' → 表达式默认值 DEFAULT ('') / DEFAULT ('[]')（8.0.13+）。

-- 渠道表
CREATE TABLE IF NOT EXISTS channels (
    id          BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name        VARCHAR(100) NOT NULL,
    description TEXT DEFAULT (''),
    status      VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- 渠道名称唯一索引
CREATE UNIQUE INDEX idx_channels_name ON channels (name);
CREATE INDEX idx_channels_status ON channels (status);

-- 渠道-分组关联表（每个分组只能属于一个渠道）
CREATE TABLE IF NOT EXISTS channel_groups (
    id          BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    channel_id  BIGINT NOT NULL,
    group_id    BIGINT NOT NULL,
    created_at  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT fk_channel_groups_channel_id FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
    CONSTRAINT fk_channel_groups_group_id FOREIGN KEY (group_id) REFERENCES `groups`(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE UNIQUE INDEX idx_channel_groups_group_id ON channel_groups (group_id);
CREATE INDEX idx_channel_groups_channel_id ON channel_groups (channel_id);

-- 渠道模型定价表（一条定价可绑定多个模型）
CREATE TABLE IF NOT EXISTS channel_model_pricing (
    id                 BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    channel_id         BIGINT NOT NULL,
    models             JSON NOT NULL DEFAULT ('[]'),
    input_price        DECIMAL(20,12),
    output_price       DECIMAL(20,12),
    cache_write_price  DECIMAL(20,12),
    cache_read_price   DECIMAL(20,12),
    image_output_price DECIMAL(20,8),
    created_at         DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at         DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT fk_channel_model_pricing_channel_id FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_channel_model_pricing_channel_id ON channel_model_pricing (channel_id);

-- (comment skipped) COMMENT ON TABLE channels
-- (comment skipped) COMMENT ON TABLE channel_groups
-- (comment skipped) COMMENT ON TABLE channel_model_pricing
-- (comment skipped) COMMENT ON COLUMN channel_model_pricing.models
-- (comment skipped) COMMENT ON COLUMN channel_model_pricing.input_price
-- (comment skipped) COMMENT ON COLUMN channel_model_pricing.output_price
-- (comment skipped) COMMENT ON COLUMN channel_model_pricing.cache_write_price
-- (comment skipped) COMMENT ON COLUMN channel_model_pricing.cache_read_price
-- (comment skipped) COMMENT ON COLUMN channel_model_pricing.image_output_price
