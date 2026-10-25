-- MySQL 方言版本（翻译自 ../070_add_usage_log_service_tier.sql）

ALTER TABLE usage_logs
    ADD COLUMN service_tier VARCHAR(16);

CREATE INDEX idx_usage_logs_service_tier_created_at
    ON usage_logs (service_tier, created_at);
