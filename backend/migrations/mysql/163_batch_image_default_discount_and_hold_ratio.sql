-- MySQL 方言版本（翻译自 ../163_batch_image_default_discount_and_hold_ratio.sql）
-- 翻译说明：
--   - ALTER COLUMN x SET DEFAULT → MySQL 语法相同，保留。
--   - COMMENT ON COLUMN → 整句跳过。
--   - `groups` 是 MySQL 8 保留字，加反引号。

ALTER TABLE `groups`
    ALTER COLUMN batch_image_discount_multiplier SET DEFAULT 0.5,
    ALTER COLUMN batch_image_hold_multiplier SET DEFAULT 0.6;

UPDATE `groups`
SET batch_image_discount_multiplier = 0.5
WHERE batch_image_discount_multiplier = 1.0;

UPDATE `groups`
SET batch_image_hold_multiplier = 0.6
WHERE batch_image_hold_multiplier = 1.05;

-- (comment skipped)

ALTER TABLE batch_image_jobs
    ALTER COLUMN batch_discount_multiplier SET DEFAULT 0.5,
    ALTER COLUMN hold_multiplier SET DEFAULT 0.6;

-- (comment skipped)
