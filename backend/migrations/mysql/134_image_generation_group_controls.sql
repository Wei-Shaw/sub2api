-- MySQL 方言版本（翻译自 ../134_image_generation_group_controls.sql）
-- 翻译说明：
--   - `groups` 为 MySQL 8 保留字，全程反引号。
--   - ADD COLUMN IF NOT EXISTS → ADD COLUMN。
--   - COMMENT ON COLUMN 整句跳过。

-- 生图能力与图片倍率模式控制
-- 兼容性原则：
-- 1. 不改写现有 image_price_1k/2k/4k，避免改变已配置分组的最终图片价格。
-- 2. 现有 openai/gemini/antigravity 分组默认保持可生图，避免升级后中断已有图片业务。
-- 3. 现有分组默认共享当前有效分组倍率，保持历史扣费公式。

ALTER TABLE `groups`
    ADD COLUMN allow_image_generation BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE `groups`
    ADD COLUMN image_rate_independent BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE `groups`
    ADD COLUMN image_rate_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0;

UPDATE `groups`
SET allow_image_generation = TRUE
WHERE platform IN ('openai', 'gemini', 'antigravity');

UPDATE `groups`
SET image_rate_independent = FALSE,
    image_rate_multiplier = 1.0;

-- (comment skipped)
-- (comment skipped)
-- (comment skipped)
