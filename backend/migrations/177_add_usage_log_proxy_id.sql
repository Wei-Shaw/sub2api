-- Preserve the effective proxy used by each request even if the account's proxy changes later.
-- This is intentionally not a foreign key: deleting a proxy must not erase or block history.
ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS proxy_id BIGINT;

COMMENT ON COLUMN usage_logs.proxy_id IS
    'Effective request-time proxy ID snapshot; NULL means direct connection or historical data';
