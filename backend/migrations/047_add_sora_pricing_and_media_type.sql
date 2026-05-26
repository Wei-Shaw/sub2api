-- Migration: 047_add_sora_pricing_and_media_type
-- 新增 Sora 按次计费字段与 usage_logs.media_type

ALTER TABLE groups
	ADD COLUMN IF NOT EXISTS sora_image_price_360 decimal(20,8),
	ADD COLUMN IF NOT EXISTS sora_image_price_540 decimal(20,8),
	ADD COLUMN IF NOT EXISTS sora_video_price_per_request decimal(20,8),
	ADD COLUMN IF NOT EXISTS sora_video_price_per_request_hd decimal(20,8);

ALTER TABLE usage_logs
	ADD COLUMN IF NOT EXISTS media_type VARCHAR(16);
