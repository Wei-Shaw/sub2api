ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS upstream_response_model VARCHAR(200);
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS upstream_model_mismatch BOOLEAN;
