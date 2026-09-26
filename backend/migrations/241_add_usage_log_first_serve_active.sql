ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS first_serve_active BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN usage_logs.first_serve_active IS
    'Request-time snapshot: streaming request reused a first-serve routing ID and proxy combination';
