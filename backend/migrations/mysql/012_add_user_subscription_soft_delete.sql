-- MySQL 方言版本（翻译自 ../012_add_user_subscription_soft_delete.sql）
-- 012: 为 user_subscriptions 表添加软删除支持
-- 任务：fix-medium-data-hygiene 1.1
--
-- 翻译说明：TIMESTAMPTZ → DATETIME(3)；CREATE INDEX IF NOT EXISTS → 去掉修饰词；
--           COMMENT ON COLUMN → 整句跳过。

-- 添加 deleted_at 字段
ALTER TABLE user_subscriptions
ADD COLUMN deleted_at DATETIME(3) DEFAULT NULL;

-- 添加 deleted_at 索引以优化软删除查询
CREATE INDEX usersubscription_deleted_at
ON user_subscriptions (deleted_at);

-- 注释：与其他使用软删除的实体保持一致
-- (comment skipped) COMMENT ON COLUMN user_subscriptions.deleted_at IS '软删除时间戳，NULL 表示未删除'
