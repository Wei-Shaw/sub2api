ALTER TABLE users
    ADD COLUMN IF NOT EXISTS trial_balance DECIMAL(20,8) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS trial_balance_expires_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS registration_ip VARCHAR(64) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_users_registration_ip
    ON users (registration_ip)
    WHERE deleted_at IS NULL;
