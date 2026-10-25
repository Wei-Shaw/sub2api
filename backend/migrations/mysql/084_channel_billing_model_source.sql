-- MySQL 方言版本（翻译自 ../084_channel_billing_model_source.sql）
-- Add billing_model_source to channels (controls whether billing uses requested or upstream model)
ALTER TABLE channels ADD COLUMN billing_model_source VARCHAR(20) DEFAULT 'requested';

-- Add channel tracking fields to usage_logs
ALTER TABLE usage_logs ADD COLUMN channel_id BIGINT;
ALTER TABLE usage_logs ADD COLUMN model_mapping_chain VARCHAR(500);
ALTER TABLE usage_logs ADD COLUMN billing_tier VARCHAR(50);
