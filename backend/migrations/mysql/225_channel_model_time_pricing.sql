-- MySQL 方言版本（翻译自 ../225_channel_model_time_pricing.sql）
-- 翻译说明：JSONB → JSON（可空）；COMMENT ON COLUMN 整句跳过。

ALTER TABLE channel_model_pricing
    ADD COLUMN time_pricing JSON NULL;

-- (comment skipped)
