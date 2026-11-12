ALTER TABLE accounts ADD COLUMN IF NOT EXISTS preferred_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_accounts_preferred_user_id ON accounts (preferred_user_id) WHERE preferred_user_id IS NOT NULL;

ALTER TABLE groups ADD COLUMN IF NOT EXISTS prefer_user_accounts BOOLEAN NOT NULL DEFAULT FALSE;
