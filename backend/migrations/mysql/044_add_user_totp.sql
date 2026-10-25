-- MySQL 方言版本（翻译自 ../044_add_user_totp.sql）
-- 为 users 表添加 TOTP 双因素认证字段
ALTER TABLE users
  ADD COLUMN totp_secret_encrypted TEXT NULL,
  ADD COLUMN totp_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN totp_enabled_at DATETIME(3) NULL;

-- (comment skipped: users.totp_secret_encrypted)
-- (comment skipped: users.totp_enabled)
-- (comment skipped: users.totp_enabled_at)

-- 创建索引以支持快速查询启用 2FA 的用户
-- （PG 版为部分索引 WHERE deleted_at IS NULL AND totp_enabled = true；非唯一 → 去掉 WHERE）
CREATE INDEX idx_users_totp_enabled ON users(totp_enabled);
