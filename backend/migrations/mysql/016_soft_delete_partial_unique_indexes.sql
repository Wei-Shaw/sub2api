-- MySQL 方言版本（翻译自 ../016_soft_delete_partial_unique_indexes.sql）
-- 修复软删除 + 唯一约束冲突问题
-- 将普通唯一约束替换为函数唯一索引（IF(deleted_at IS NULL, 0, NULL) 键部分），
-- 软删除的记录不占用唯一性位置，允许删后重建同名/同邮箱/同订阅关系。
--
-- 索引名说明：MySQL 的 CREATE TABLE 内联 UNIQUE 生成的索引名 = 首列列名：
--   users.email                        → 索引 `email`（001_init.sql）
--   `groups`.name                      → 索引 `name`（001_init.sql）
--   user_subscriptions(user_id, group_id) → 索引 `user_id`（003_subscription.sql）

-- ============================================================================
-- 1. users 表: email 字段
-- ============================================================================

-- 删除旧的唯一索引（MySQL 内联 UNIQUE 生成的索引名=列名 `email`）
ALTER TABLE users DROP INDEX email;

-- 创建函数唯一索引：只对未删除的记录参与唯一性约束
CREATE UNIQUE INDEX users_email_unique_active
    ON users(email, (IF(deleted_at IS NULL, 0, NULL)));

-- ============================================================================
-- 2. groups 表: name 字段
-- ============================================================================

-- 删除旧的唯一索引（索引名=列名 `name`）
ALTER TABLE `groups` DROP INDEX name;

-- 创建函数唯一索引
CREATE UNIQUE INDEX groups_name_unique_active
    ON `groups`(name, (IF(deleted_at IS NULL, 0, NULL)));

-- ============================================================================
-- 3. user_subscriptions 表: (user_id, group_id) 组合字段
-- ============================================================================

-- 删除旧的唯一索引（UNIQUE(user_id, group_id) 生成的索引名=首列 `user_id`；
-- 003 中另有普通索引 idx_user_subscriptions_user_id，删除后 FK 仍可使用它）
ALTER TABLE user_subscriptions DROP INDEX user_id;

-- 创建函数唯一索引
CREATE UNIQUE INDEX user_subscriptions_user_group_unique_active
    ON user_subscriptions(user_id, group_id, (IF(deleted_at IS NULL, 0, NULL)));

-- ============================================================================
-- 注意: api_keys 表的 key 字段保留普通唯一约束
-- API Key 即使软删除后也不应该重复使用（安全考虑）
-- ============================================================================
