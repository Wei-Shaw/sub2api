ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS service_tier VARCHAR(16);

CREATE INDEX IF NOT EXISTS idx_usage_logs_service_tier_created_at
    ON usage_logs (service_tier, created_at);
