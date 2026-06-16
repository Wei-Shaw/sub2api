-- 016_soft_delete_partial_unique_indexes.sql
-- 修复软删除 + 唯一约束冲突问题
-- 将普通唯一约束替换为部分唯一索引（WHERE deleted_at IS NULL）
-- 这样软删除的记录不会占用唯一约束位置，允许删后重建同名/同邮箱/同订阅关系

-- ============================================================================
-- 1. users 表: email 字段
-- ============================================================================

-- 删除旧的唯一约束（可能的命名方式）
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_key;
DROP INDEX IF EXISTS users_email_key;
DROP INDEX IF EXISTS user_email_key;

-- 创建部分唯一索引：只对未删除的记录建立唯一约束
CREATE UNIQUE INDEX IF NOT EXISTS users_email_unique_active
    ON users(email)
    WHERE deleted_at IS NULL;

-- ============================================================================
-- 2. groups 表: name 字段
-- ============================================================================

-- 删除旧的唯一约束
ALTER TABLE groups DROP CONSTRAINT IF EXISTS groups_name_key;
DROP INDEX IF EXISTS groups_name_key;
DROP INDEX IF EXISTS group_name_key;

-- 创建部分唯一索引
CREATE UNIQUE INDEX IF NOT EXISTS groups_name_unique_active
    ON groups(name)
    WHERE deleted_at IS NULL;

-- ============================================================================
-- 3. user_subscriptions 表: (user_id, group_id) 组合字段
-- ============================================================================

-- 删除旧的唯一约束/索引
ALTER TABLE user_subscriptions DROP CONSTRAINT IF EXISTS user_subscriptions_user_id_group_id_key;
DROP INDEX IF EXISTS user_subscriptions_user_id_group_id_key;
DROP INDEX IF EXISTS usersubscription_user_id_group_id;

-- 创建部分唯一索引
CREATE UNIQUE INDEX IF NOT EXISTS user_subscriptions_user_group_unique_active
    ON user_subscriptions(user_id, group_id)
    WHERE deleted_at IS NULL;

-- ============================================================================
-- 注意: api_keys 表的 key 字段保留普通唯一约束
-- API Key 即使软删除后也不应该重复使用（安全考虑）
-- ============================================================================
