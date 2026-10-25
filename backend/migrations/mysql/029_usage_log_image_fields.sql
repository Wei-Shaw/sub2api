-- MySQL 方言版本（翻译自 ../029_usage_log_image_fields.sql）
-- 为使用日志添加图片生成统计字段
-- 用于记录 gemini-3-pro-image 等图片生成模型的使用情况
--
-- 翻译说明：ADD COLUMN IF NOT EXISTS → 去掉修饰词。

ALTER TABLE usage_logs ADD COLUMN image_count INT DEFAULT 0;
ALTER TABLE usage_logs ADD COLUMN image_size VARCHAR(10);
