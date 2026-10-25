-- MySQL 方言版本（翻译自 ../047_add_sora_pricing_and_media_type.sql）
-- Migration: 047_add_sora_pricing_and_media_type
-- 新增 Sora 按次计费字段与 usage_logs.media_type
-- （`groups` 为 MySQL 8 保留字，加反引号）

ALTER TABLE `groups`
    ADD COLUMN sora_image_price_360 DECIMAL(20,8) NULL,
    ADD COLUMN sora_image_price_540 DECIMAL(20,8) NULL,
    ADD COLUMN sora_video_price_per_request DECIMAL(20,8) NULL,
    ADD COLUMN sora_video_price_per_request_hd DECIMAL(20,8) NULL;

ALTER TABLE usage_logs
    ADD COLUMN media_type VARCHAR(16) NULL;
