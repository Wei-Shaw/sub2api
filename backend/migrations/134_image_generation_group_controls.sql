-- 生图能力与图片倍率模式控制
-- 兼容性原则：
-- 1. 不改写现有 image_price_1k/2k/4k，避免改变已配置分组的最终图片价格。
-- 2. 现有 openai/gemini/antigravity 分组默认保持可生图，避免升级后中断已有图片业务。
-- 3. 现有分组默认共享当前有效分组倍率，保持历史扣费公式。

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS allow_image_generation BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS image_rate_independent BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS image_rate_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0;

UPDATE groups
SET allow_image_generation = true
WHERE platform IN ('openai', 'gemini', 'antigravity');

UPDATE groups
SET image_rate_independent = false,
    image_rate_multiplier = 1.0;

COMMENT ON COLUMN groups.allow_image_generation IS '是否允许该分组使用图片生成能力';
COMMENT ON COLUMN groups.image_rate_independent IS '图片生成是否使用独立倍率；false 表示共享分组有效倍率';
COMMENT ON COLUMN groups.image_rate_multiplier IS '图片生成独立倍率，仅 image_rate_independent=true 时生效';
