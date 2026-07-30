-- 用户自建账号：owner / visibility / upstream_plan；共享池组标记 is_share_pool
ALTER TABLE accounts
  ADD COLUMN IF NOT EXISTS owner_user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS visibility VARCHAR(16) NULL,
  ADD COLUMN IF NOT EXISTS upstream_plan VARCHAR(64) NULL;

CREATE INDEX IF NOT EXISTS idx_accounts_owner_user_id
  ON accounts (owner_user_id)
  WHERE deleted_at IS NULL AND owner_user_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_accounts_owner_visibility
  ON accounts (owner_user_id, visibility)
  WHERE deleted_at IS NULL AND owner_user_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_accounts_public_plan
  ON accounts (platform, upstream_plan)
  WHERE deleted_at IS NULL AND visibility = 'public' AND owner_user_id IS NOT NULL;

ALTER TABLE groups
  ADD COLUMN IF NOT EXISTS is_share_pool BOOLEAN NOT NULL DEFAULT FALSE;
