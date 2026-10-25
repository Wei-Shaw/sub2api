-- MySQL 方言版本（翻译自 ../161_batch_image_pricing_snapshot.sql）
-- 翻译说明：
--   - ADD COLUMN IF NOT EXISTS → ADD COLUMN（IF NOT EXISTS 按迁移执行器单次执行语义去掉）。
--   - COMMENT ON COLUMN → 整句跳过（MySQL 需整列定义重建，风险大于收益）。
--   - INTEGER → INT。

ALTER TABLE `groups`
    ADD COLUMN batch_image_discount_multiplier DECIMAL(10,4) NOT NULL DEFAULT 0.5,
    ADD COLUMN batch_image_hold_multiplier DECIMAL(10,4) NOT NULL DEFAULT 0.6;

-- (comment skipped)
-- (comment skipped)

ALTER TABLE batch_image_jobs
    ADD COLUMN base_unit_price DECIMAL(20,10) NOT NULL DEFAULT 0,
    ADD COLUMN group_rate_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0,
    ADD COLUMN account_rate_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0,
    ADD COLUMN batch_discount_multiplier DECIMAL(10,4) NOT NULL DEFAULT 0.5,
    ADD COLUMN hold_multiplier DECIMAL(10,4) NOT NULL DEFAULT 0.6,
    ADD COLUMN billable_unit_price DECIMAL(20,10) NOT NULL DEFAULT 0,
    ADD COLUMN hold_unit_price DECIMAL(20,10) NOT NULL DEFAULT 0,
    ADD COLUMN pricing_snapshot_version INT NOT NULL DEFAULT 0;

-- (comment skipped)
-- (comment skipped)
-- (comment skipped)
-- (comment skipped)
-- (comment skipped)
-- (comment skipped)
-- (comment skipped)
-- (comment skipped)
