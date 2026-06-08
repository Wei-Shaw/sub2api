-- 为使用日志添加图片生成统计字段
-- 用于记录 gemini-3-pro-image 等图片生成模型的使用情况

ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS image_count INT DEFAULT 0;
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS image_size VARCHAR(10);
