ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS proxy_snapshot JSONB;
COMMENT ON COLUMN usage_logs.proxy_snapshot IS 'Request-time proxy identity and observed exit IP snapshot; NULL for historical records';
