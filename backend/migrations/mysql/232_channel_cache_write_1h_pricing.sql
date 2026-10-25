-- MySQL 方言版本（翻译自 ../232_channel_cache_write_1h_pricing.sql）
-- 翻译说明：NUMERIC(20,12) → DECIMAL(20,12)（同义类型）；
--           ADD COLUMN IF NOT EXISTS → ADD COLUMN；COMMENT ON COLUMN 整句跳过。

ALTER TABLE channel_model_pricing
    ADD COLUMN cache_write_1h_price DECIMAL(20,12) NULL;

ALTER TABLE channel_pricing_intervals
    ADD COLUMN cache_write_1h_price DECIMAL(20,12) NULL;

ALTER TABLE channel_account_stats_model_pricing
    ADD COLUMN cache_write_1h_price DECIMAL(20,12) NULL;

ALTER TABLE channel_account_stats_pricing_intervals
    ADD COLUMN cache_write_1h_price DECIMAL(20,12) NULL;

-- (comment skipped)
-- (comment skipped)
-- (comment skipped)
-- (comment skipped)
