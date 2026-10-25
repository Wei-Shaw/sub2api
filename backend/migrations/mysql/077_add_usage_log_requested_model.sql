-- MySQL 方言版本（翻译自 ../077_add_usage_log_requested_model.sql）
-- Add requested_model field to usage_logs for normalized request/upstream model tracking.
-- NULL means historical rows written before requested_model dual-write was introduced.

ALTER TABLE usage_logs ADD COLUMN requested_model VARCHAR(100);
