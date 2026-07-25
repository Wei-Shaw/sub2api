ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS auto_group_id BIGINT;

CREATE INDEX IF NOT EXISTS idx_usage_logs_auto_group_created_at
    ON usage_logs (auto_group_id, created_at DESC)
    WHERE auto_group_id IS NOT NULL;
