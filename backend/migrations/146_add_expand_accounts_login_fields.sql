ALTER TABLE IF EXISTS expand_accounts
    ADD COLUMN IF NOT EXISTS account_id BIGINT;

ALTER TABLE IF EXISTS expand_accounts
    ADD COLUMN IF NOT EXISTS login_status BIGINT NOT NULL DEFAULT 0;

ALTER TABLE IF EXISTS expand_accounts
    ADD COLUMN IF NOT EXISTS device_id TEXT;

ALTER TABLE IF EXISTS expand_accounts
    ADD COLUMN IF NOT EXISTS api_key TEXT;

CREATE INDEX IF NOT EXISTS idx_expand_accounts_account_id ON expand_accounts(account_id);
CREATE INDEX IF NOT EXISTS idx_expand_accounts_login_status ON expand_accounts(login_status);
