-- 012: 为 user_subscriptions 表添加软删除支持
-- 任务：fix-medium-data-hygiene 1.1

-- 添加 deleted_at 字段
ALTER TABLE user_subscriptions
ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ DEFAULT NULL;

-- 添加 deleted_at 索引以优化软删除查询
CREATE INDEX IF NOT EXISTS usersubscription_deleted_at
ON user_subscriptions (deleted_at);

-- 注释：与其他使用软删除的实体保持一致
COMMENT ON COLUMN user_subscriptions.deleted_at IS '软删除时间戳，NULL 表示未删除';
