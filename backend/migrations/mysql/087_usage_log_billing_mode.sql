-- MySQL 方言版本（翻译自 ../087_usage_log_billing_mode.sql）
-- Add billing_mode to usage_logs (records the billing mode: token/per_request/image)
ALTER TABLE usage_logs ADD COLUMN billing_mode VARCHAR(20);
