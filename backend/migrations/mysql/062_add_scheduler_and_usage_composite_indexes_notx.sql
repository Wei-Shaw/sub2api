-- MySQL 方言版本（翻译自 ../062_add_scheduler_and_usage_composite_indexes_notx.sql）
-- 说明：
-- - 保留 _notx 文件名；MySQL 的 CREATE INDEX 无 CONCURRENTLY，直接去掉；
-- - PG 版四个索引均为非唯一部分索引 → 去掉 WHERE 谓词（普通索引，仅性能差异）。

CREATE INDEX idx_accounts_schedulable_hot
    ON accounts (platform, priority);

CREATE INDEX idx_accounts_active_schedulable
    ON accounts (priority, status);

CREATE INDEX idx_user_subscriptions_user_status_expires_active
    ON user_subscriptions (user_id, status, expires_at);

CREATE INDEX idx_usage_logs_group_created_at_not_null
    ON usage_logs (group_id, created_at);
