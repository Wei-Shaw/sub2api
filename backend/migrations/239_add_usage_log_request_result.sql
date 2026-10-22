-- NULL preserves the unknown outcome of historical records. Token values alone
-- cannot distinguish missing usage from an explicitly reported zero.
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS request_result JSONB;
