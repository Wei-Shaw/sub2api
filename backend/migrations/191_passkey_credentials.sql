CREATE TABLE IF NOT EXISTS passkey_user_handles (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    user_handle BYTEA NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS passkey_credentials (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id BYTEA NOT NULL UNIQUE,
    name VARCHAR(100) NOT NULL DEFAULT 'Passkey',
    credential_data JSONB NOT NULL,
    last_used_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS passkey_credentials_user_id_idx
    ON passkey_credentials (user_id);

CREATE INDEX IF NOT EXISTS passkey_credentials_last_used_at_idx
    ON passkey_credentials (last_used_at);
