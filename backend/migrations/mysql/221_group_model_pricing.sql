-- MySQL 方言版本（翻译自 ../221_group_model_pricing.sql）
-- 翻译说明：`groups` 为 MySQL 8 保留字需反引号；JSONB → JSON；
--           `IS DISTINCT FROM TRUE` → `NOT (col <=> TRUE)`；COMMENT ON COLUMN 整句跳过。

ALTER TABLE `groups`
    ADD COLUMN long_context_pricing_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN model_pricing JSON NULL;

UPDATE `groups`
SET long_context_pricing_enabled = TRUE
WHERE NOT (long_context_pricing_enabled <=> TRUE);

-- (comment skipped)
-- (comment skipped)
