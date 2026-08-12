-- Per-proxy HTTP transport controls. Both options are opt-in so existing
-- proxies retain their current connection behavior after the migration.
ALTER TABLE proxies
    ADD COLUMN IF NOT EXISTS force_http1 BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS disable_keep_alive BOOLEAN NOT NULL DEFAULT FALSE;
