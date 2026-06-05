ALTER TABLE service_quota_rules ADD COLUMN IF NOT EXISTS channel_id bigint NULL REFERENCES channels(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_service_quota_rules_channel_id ON service_quota_rules(channel_id) WHERE channel_id IS NOT NULL;

-- 重建唯一索引以包含 channel_id 维度
DROP INDEX IF EXISTS idx_service_quota_rules_unique;
CREATE UNIQUE INDEX idx_service_quota_rules_unique ON service_quota_rules (
    COALESCE(platform, ''),
    COALESCE(channel_id, 0),
    COALESCE(group_id, 0),
    COALESCE(account_id, 0),
    COALESCE(model_pattern, ''),
    limiter_type, counter_mode, is_fallback
);
