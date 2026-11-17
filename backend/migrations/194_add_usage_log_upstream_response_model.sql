ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS upstream_response_model VARCHAR(200),
    ADD COLUMN IF NOT EXISTS upstream_model_mismatch BOOLEAN;
