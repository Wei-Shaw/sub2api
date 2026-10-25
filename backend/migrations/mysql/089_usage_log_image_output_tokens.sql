-- MySQL 方言版本（翻译自 ../089_usage_log_image_output_tokens.sql）

ALTER TABLE usage_logs ADD COLUMN image_output_tokens INT NOT NULL DEFAULT 0;
ALTER TABLE usage_logs ADD COLUMN image_output_cost DECIMAL(20, 10) NOT NULL DEFAULT 0;
