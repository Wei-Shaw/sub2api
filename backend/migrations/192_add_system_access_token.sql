-- System access token: long-lived PAT for management API (like new-api's 访问令牌).
-- Stored as SHA-256 hex digest; cleartext is shown once at creation (sat_<hex>).
ALTER TABLE users ADD COLUMN IF NOT EXISTS system_token_hash VARCHAR(64);

-- Lookup index: only live users with a token set.
CREATE INDEX IF NOT EXISTS idx_users_system_token_hash
    ON users (system_token_hash)
    WHERE system_token_hash IS NOT NULL AND deleted_at IS NULL;
