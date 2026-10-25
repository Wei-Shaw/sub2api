-- MySQL 方言版本（翻译自 ../082_refactor_channel_pricing.sql）
-- Extend channel_model_pricing with billing_mode and add context-interval child table.
-- Supports three billing modes: token (per-token with context intervals),
-- per_request (per-request with context-size tiers), and image (per-image).
--
-- 翻译说明：PG 的 SET LOCAL lock_timeout / statement_timeout 无 MySQL 等价语句，跳过。

-- 1. 为 channel_model_pricing 添加 billing_mode 列
ALTER TABLE channel_model_pricing
    ADD COLUMN billing_mode VARCHAR(20) NOT NULL DEFAULT 'token';

-- (comment skipped) COMMENT ON COLUMN channel_model_pricing.billing_mode

-- 2. 创建区间定价子表
CREATE TABLE IF NOT EXISTS channel_pricing_intervals (
    id                BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    pricing_id        BIGINT NOT NULL,
    min_tokens        INT NOT NULL DEFAULT 0,
    max_tokens        INT,
    tier_label        VARCHAR(50),
    input_price       DECIMAL(20,12),
    output_price      DECIMAL(20,12),
    cache_write_price DECIMAL(20,12),
    cache_read_price  DECIMAL(20,12),
    per_request_price DECIMAL(20,12),
    sort_order        INT NOT NULL DEFAULT 0,
    created_at        DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at        DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT fk_channel_pricing_intervals_pricing_id FOREIGN KEY (pricing_id) REFERENCES channel_model_pricing(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_channel_pricing_intervals_pricing_id
    ON channel_pricing_intervals (pricing_id);

-- (comment skipped) COMMENT ON TABLE channel_pricing_intervals
-- (comment skipped) COMMENT ON COLUMN channel_pricing_intervals.min_tokens
-- (comment skipped) COMMENT ON COLUMN channel_pricing_intervals.max_tokens
-- (comment skipped) COMMENT ON COLUMN channel_pricing_intervals.tier_label
-- (comment skipped) COMMENT ON COLUMN channel_pricing_intervals.input_price
-- (comment skipped) COMMENT ON COLUMN channel_pricing_intervals.output_price
-- (comment skipped) COMMENT ON COLUMN channel_pricing_intervals.cache_write_price
-- (comment skipped) COMMENT ON COLUMN channel_pricing_intervals.cache_read_price
-- (comment skipped) COMMENT ON COLUMN channel_pricing_intervals.per_request_price

-- 3. 迁移现有 flat 定价为单区间 [0, +inf)
-- 仅迁移有明确定价（至少一个价格字段非 NULL）的条目
-- 翻译说明：INSERT ... SELECT ... WHERE NOT EXISTS（子查询引用插入目标表）在 MySQL 中为
--           标准防重插入模式，语义与 PG 一致。
INSERT INTO channel_pricing_intervals (pricing_id, min_tokens, max_tokens, input_price, output_price, cache_write_price, cache_read_price, sort_order)
SELECT
    cmp.id,
    0,
    NULL,
    cmp.input_price,
    cmp.output_price,
    cmp.cache_write_price,
    cmp.cache_read_price,
    0
FROM channel_model_pricing cmp
WHERE cmp.billing_mode = 'token'
  AND (cmp.input_price IS NOT NULL OR cmp.output_price IS NOT NULL
       OR cmp.cache_write_price IS NOT NULL OR cmp.cache_read_price IS NOT NULL)
  AND NOT EXISTS (
      SELECT 1 FROM channel_pricing_intervals cpi WHERE cpi.pricing_id = cmp.id
  );

-- 4. 迁移 image_output_price 为 image 模式的区间条目
-- 将有 image_output_price 的现有条目复制为 billing_mode='image' 的独立条目
-- 注意：这里不改变原条目的 billing_mode，而是将 image_output_price 作为向后兼容字段保留
-- 实际的 image 计费在未来由独立的 billing_mode='image' 条目处理
