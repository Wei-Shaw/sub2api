CREATE TABLE IF NOT EXISTS developer_keys (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    key_prefix VARCHAR(24) NOT NULL,
    key_hash CHAR(64) NOT NULL UNIQUE,
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS developer_keys_user_id_created_at_idx
    ON developer_keys (user_id, created_at DESC);

COMMENT ON TABLE developer_keys IS
    'User-owned credentials restricted to developer file APIs; plaintext secrets are never stored.';
COMMENT ON COLUMN developer_keys.key_prefix IS
    'Non-secret display prefix for identifying a key in the console.';
COMMENT ON COLUMN developer_keys.key_hash IS
    'Lowercase SHA-256 hex digest of the complete developer key.';
