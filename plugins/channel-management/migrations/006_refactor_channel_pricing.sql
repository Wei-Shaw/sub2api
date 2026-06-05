-- Extend channel_model_pricing with billing_mode and add context-interval child table.
-- Supports three billing modes: token (per-token with context intervals),
-- per_request (per-request with context-size tiers), and image (per-image).

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

-- 1. 为 channel_model_pricing 添加 billing_mode 列
ALTER TABLE channel_model_pricing
    ADD COLUMN IF NOT EXISTS billing_mode VARCHAR(20) NOT NULL DEFAULT 'token';

COMMENT ON COLUMN channel_model_pricing.billing_mode IS '计费模式：token（按 token 区间计费）、per_request（按次计费）、image（图片计费）';

-- 2. 创建区间定价子表
CREATE TABLE IF NOT EXISTS channel_pricing_intervals (
    id                BIGSERIAL      PRIMARY KEY,
    pricing_id        BIGINT         NOT NULL REFERENCES channel_model_pricing(id) ON DELETE CASCADE,
    min_tokens        INT            NOT NULL DEFAULT 0,
    max_tokens        INT,
    tier_label        VARCHAR(50),
    input_price       NUMERIC(20,12),
    output_price      NUMERIC(20,12),
    cache_write_price NUMERIC(20,12),
    cache_read_price  NUMERIC(20,12),
    per_request_price NUMERIC(20,12),
    sort_order        INT            NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_channel_pricing_intervals_pricing_id
    ON channel_pricing_intervals (pricing_id);

COMMENT ON TABLE channel_pricing_intervals IS '渠道定价区间：支持按 token 区间、按次分层、图片分辨率分层';
COMMENT ON COLUMN channel_pricing_intervals.min_tokens IS '区间下界（含），token 模式使用';
COMMENT ON COLUMN channel_pricing_intervals.max_tokens IS '区间上界（不含），NULL 表示无上限';
COMMENT ON COLUMN channel_pricing_intervals.tier_label IS '层级标签，按次/图片模式使用（如 1K、2K、4K、HD）';
COMMENT ON COLUMN channel_pricing_intervals.input_price IS 'token 模式：每 token 输入价';
COMMENT ON COLUMN channel_pricing_intervals.output_price IS 'token 模式：每 token 输出价';
COMMENT ON COLUMN channel_pricing_intervals.cache_write_price IS 'token 模式：缓存写入价';
COMMENT ON COLUMN channel_pricing_intervals.cache_read_price IS 'token 模式：缓存读取价';
COMMENT ON COLUMN channel_pricing_intervals.per_request_price IS '按次/图片模式：每次请求价格';

-- 3. 迁移现有 flat 定价为单区间 [0, +inf)
-- 仅迁移有明确定价（至少一个价格字段非 NULL）的条目
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
