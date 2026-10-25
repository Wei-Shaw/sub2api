-- MySQL 方言版本（翻译自 ../170_add_grok_video_pricing_controls.sql）
-- 翻译说明：
--   - ADD COLUMN IF NOT EXISTS → ADD COLUMN（IF NOT EXISTS 按迁移执行器单次执行语义去掉）。
--   - 布尔字面量 false → FALSE。
--   - COMMENT ON COLUMN → 整句跳过。

-- Add independent group pricing controls for Grok video generation.
-- Video prices intentionally do not backfill from image prices: image and video
-- generation must be priced separately.

ALTER TABLE `groups`
    ADD COLUMN video_rate_independent BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN video_rate_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0,
    ADD COLUMN video_price_480p DECIMAL(20,8) NULL,
    ADD COLUMN video_price_720p DECIMAL(20,8) NULL,
    ADD COLUMN video_price_1080p DECIMAL(20,8) NULL;

-- (comment skipped)
-- (comment skipped)
-- (comment skipped)
-- (comment skipped)
-- (comment skipped)
