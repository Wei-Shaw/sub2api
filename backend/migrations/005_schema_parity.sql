-- Align SQL migrations with current GORM persistence models.
-- This file is designed to be safe on both fresh installs and existing databases.

-- users: add fields added after initial migration
ALTER TABLE users ADD COLUMN IF NOT EXISTS username VARCHAR(100) NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS wechat VARCHAR(100) NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS notes TEXT NOT NULL DEFAULT '';

-- api_keys: allow longer keys (GORM model uses size:128)
ALTER TABLE api_keys ALTER COLUMN key TYPE VARCHAR(128);

-- accounts: scheduling and rate-limit fields used by repository queries
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS schedulable BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS rate_limited_at TIMESTAMPTZ;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS rate_limit_reset_at TIMESTAMPTZ;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS overload_until TIMESTAMPTZ;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS session_window_start TIMESTAMPTZ;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS session_window_end TIMESTAMPTZ;
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS session_window_status VARCHAR(20);

CREATE INDEX IF NOT EXISTS idx_accounts_schedulable ON accounts(schedulable);
CREATE INDEX IF NOT EXISTS idx_accounts_rate_limited_at ON accounts(rate_limited_at);
CREATE INDEX IF NOT EXISTS idx_accounts_rate_limit_reset_at ON accounts(rate_limit_reset_at);
CREATE INDEX IF NOT EXISTS idx_accounts_overload_until ON accounts(overload_until);

-- redeem_codes: subscription redeem fields
ALTER TABLE redeem_codes ADD COLUMN IF NOT EXISTS group_id BIGINT REFERENCES groups(id) ON DELETE SET NULL;
ALTER TABLE redeem_codes ADD COLUMN IF NOT EXISTS validity_days INT NOT NULL DEFAULT 30;
CREATE INDEX IF NOT EXISTS idx_redeem_codes_group_id ON redeem_codes(group_id);

-- usage_logs: billing type used by filters and stats
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS billing_type SMALLINT NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_usage_logs_billing_type ON usage_logs(billing_type);

-- settings: key-value store
CREATE TABLE IF NOT EXISTS settings (
    id          BIGSERIAL PRIMARY KEY,
    key         VARCHAR(100) NOT NULL UNIQUE,
    value       TEXT NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

