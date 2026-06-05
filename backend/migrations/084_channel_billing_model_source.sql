-- Add billing_model_source to channels (controls whether billing uses requested or upstream model)
ALTER TABLE channels ADD COLUMN IF NOT EXISTS billing_model_source VARCHAR(20) DEFAULT 'requested';

-- Add channel tracking fields to usage_logs
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS channel_id BIGINT;
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS model_mapping_chain VARCHAR(500);
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS billing_tier VARCHAR(50);
