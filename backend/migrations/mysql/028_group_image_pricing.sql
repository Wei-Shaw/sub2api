-- MySQL 方言版本（翻译自 ../028_group_image_pricing.sql）
-- 为 Antigravity 分组添加图片生成计费配置
-- 支持 gemini-3-pro-image 模型的 1K/2K/4K 分辨率按次计费
--
-- 翻译说明：ADD COLUMN IF NOT EXISTS → 去掉修饰词；
--           COMMENT ON COLUMN → 整句跳过；`groups` 为 MySQL 8 保留字。

ALTER TABLE `groups` ADD COLUMN image_price_1k DECIMAL(20,8);
ALTER TABLE `groups` ADD COLUMN image_price_2k DECIMAL(20,8);
ALTER TABLE `groups` ADD COLUMN image_price_4k DECIMAL(20,8);

-- (comment skipped) COMMENT ON COLUMN groups.image_price_1k IS '1K 分辨率图片生成单价 (USD)，仅 antigravity 平台使用'
-- (comment skipped) COMMENT ON COLUMN groups.image_price_2k IS '2K 分辨率图片生成单价 (USD)，仅 antigravity 平台使用'
-- (comment skipped) COMMENT ON COLUMN groups.image_price_4k IS '4K 分辨率图片生成单价 (USD)，仅 antigravity 平台使用'
