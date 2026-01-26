-- 为 users 表添加 TOTP 双因素认证字段
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS totp_secret_encrypted TEXT DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS totp_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS totp_enabled_at TIMESTAMPTZ DEFAULT NULL;

COMMENT ON COLUMN users.totp_secret_encrypted IS 'AES-256-GCM 加密的 TOTP 密钥';
COMMENT ON COLUMN users.totp_enabled IS '是否启用 TOTP 双因素认证';
COMMENT ON COLUMN users.totp_enabled_at IS 'TOTP 启用时间';

-- 创建索引以支持快速查询启用 2FA 的用户
CREATE INDEX IF NOT EXISTS idx_users_totp_enabled ON users(totp_enabled) WHERE deleted_at IS NULL AND totp_enabled = true;
