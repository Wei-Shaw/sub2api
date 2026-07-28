CREATE INDEX CONCURRENTLY IF NOT EXISTS passkey_credentials_user_id_idx
    ON passkey_credentials (user_id);

CREATE INDEX CONCURRENTLY IF NOT EXISTS passkey_credentials_last_used_at_idx
    ON passkey_credentials (last_used_at);
