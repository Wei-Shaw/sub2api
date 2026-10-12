ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS usage_source VARCHAR(16) NOT NULL DEFAULT 'upstream',
    ADD COLUMN IF NOT EXISTS client_disconnected BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN usage_logs.usage_source IS 'Token usage source: upstream or estimated';
COMMENT ON COLUMN usage_logs.client_disconnected IS 'Whether the downstream client disconnected before upstream completion';
