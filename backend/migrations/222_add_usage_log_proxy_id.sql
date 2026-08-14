-- Persist the proxy actually used for the upstream request so usage rows
-- can be correlated by proxy without joining accounts (whose proxy_id may
-- have changed since the request).
--
-- Nullable with no default: on PostgreSQL 11+ this is a metadata-only change,
-- so it does NOT rewrite the (potentially large) usage_logs table. Historical
-- rows and requests that did not go through a proxy stay NULL. No FK: matches
-- channel_id (proxy may be deleted later; the snapshot id is still useful).
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS proxy_id BIGINT;
