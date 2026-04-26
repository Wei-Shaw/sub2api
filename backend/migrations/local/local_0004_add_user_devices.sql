CREATE TABLE IF NOT EXISTS user_devices (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_hash VARCHAR(64) NOT NULL,
    fingerprint_version INTEGER NOT NULL DEFAULT 1,
    install_id VARCHAR(128),
    platform VARCHAR(32) NOT NULL,
    arch VARCHAR(16) NOT NULL,
    app_version VARCHAR(32),
    claim_redeem_code_id BIGINT REFERENCES redeem_codes(id) ON DELETE SET NULL,
    login_redeem_code_id BIGINT NOT NULL REFERENCES redeem_codes(id) ON DELETE RESTRICT,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    first_claimed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_claimed_at TIMESTAMPTZ,
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT user_devices_status_check CHECK (status IN ('active', 'revoked', 'blocked'))
);

CREATE UNIQUE INDEX IF NOT EXISTS user_devices_device_hash_key ON user_devices(device_hash);
CREATE UNIQUE INDEX IF NOT EXISTS user_devices_claim_redeem_code_id_key ON user_devices(claim_redeem_code_id) WHERE claim_redeem_code_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS user_devices_login_redeem_code_id_key ON user_devices(login_redeem_code_id);
CREATE INDEX IF NOT EXISTS user_devices_user_id_idx ON user_devices(user_id);
CREATE INDEX IF NOT EXISTS user_devices_status_idx ON user_devices(status);
