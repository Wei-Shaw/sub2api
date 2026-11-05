-- HTTP entity-body traffic observations. NULL means unknown/unmeasured, including
-- every historical row. Do not add a default or backfill synthetic zero values.
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS traffic JSONB;

COMMENT ON COLUMN usage_logs.traffic IS
    'Observed HTTP entity-body bytes: upstream/downstream -> request/response -> original_bytes, actual_bytes, compressed; NULL/missing means unknown';
