-- MySQL 方言版本（翻译自 ../228_channel_pricing_multipliers.sql）
-- 翻译说明：NUMERIC → DECIMAL（同义类型）；ADD COLUMN IF NOT EXISTS → ADD COLUMN；
--           PG DO 块（逐个查 pg_constraint 是否已存在同名 CHECK 约束）→ 迁移执行器
--           保证单次执行，存在性分支去掉，直接添加 6 个 CHECK 约束（CHECK 约束需
--           MySQL 8.0.16+ 才强制执行）；COMMENT ON COLUMN 整句跳过。

ALTER TABLE channel_model_pricing
    ADD COLUMN fast_multiplier DECIMAL(12,6) NULL,
    ADD COLUMN flex_multiplier DECIMAL(12,6) NULL;

ALTER TABLE channel_pricing_intervals
    ADD COLUMN input_multiplier DECIMAL(12,6) NULL,
    ADD COLUMN output_multiplier DECIMAL(12,6) NULL,
    ADD COLUMN cache_write_multiplier DECIMAL(12,6) NULL,
    ADD COLUMN cache_read_multiplier DECIMAL(12,6) NULL;

ALTER TABLE channel_model_pricing
    ADD CONSTRAINT channel_model_pricing_fast_multiplier_positive
    CHECK (fast_multiplier IS NULL OR fast_multiplier > 0);

ALTER TABLE channel_model_pricing
    ADD CONSTRAINT channel_model_pricing_flex_multiplier_positive
    CHECK (flex_multiplier IS NULL OR flex_multiplier > 0);

ALTER TABLE channel_pricing_intervals
    ADD CONSTRAINT channel_pricing_intervals_input_multiplier_positive
    CHECK (input_multiplier IS NULL OR input_multiplier > 0);

ALTER TABLE channel_pricing_intervals
    ADD CONSTRAINT channel_pricing_intervals_output_multiplier_positive
    CHECK (output_multiplier IS NULL OR output_multiplier > 0);

ALTER TABLE channel_pricing_intervals
    ADD CONSTRAINT channel_pricing_intervals_cache_write_multiplier_positive
    CHECK (cache_write_multiplier IS NULL OR cache_write_multiplier > 0);

ALTER TABLE channel_pricing_intervals
    ADD CONSTRAINT channel_pricing_intervals_cache_read_multiplier_positive
    CHECK (cache_read_multiplier IS NULL OR cache_read_multiplier > 0);

-- (comment skipped)
-- (comment skipped)
-- (comment skipped)
-- (comment skipped)
-- (comment skipped)
-- (comment skipped)
