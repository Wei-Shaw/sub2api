-- MySQL 方言版本（翻译自 ../075_add_usage_log_upstream_model.sql）
-- Add upstream_model field to usage_logs.
-- Stores the actual upstream model name when it differs from the requested model
-- (i.e., when model mapping is applied). NULL means no mapping was applied.

ALTER TABLE usage_logs ADD COLUMN upstream_model VARCHAR(100);
