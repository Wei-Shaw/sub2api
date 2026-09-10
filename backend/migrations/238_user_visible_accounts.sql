-- Maintain the explicit set of users allowed to view each upstream account.
CREATE TABLE IF NOT EXISTS user_visible_accounts (
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, account_id)
);

CREATE INDEX IF NOT EXISTS idx_user_visible_accounts_account_id
    ON user_visible_accounts(account_id);
