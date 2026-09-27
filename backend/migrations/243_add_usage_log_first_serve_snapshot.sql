ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS first_serve_snapshot JSONB;

COMMENT ON COLUMN usage_logs.first_serve_snapshot IS
    'Request-time first-serve route snapshot: elapsed seconds, proxy name, and proxy address';
