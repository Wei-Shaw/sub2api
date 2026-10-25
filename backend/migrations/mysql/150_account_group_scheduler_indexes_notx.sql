-- MySQL 方言版本（翻译自 ../150_account_group_scheduler_indexes_notx.sql）
-- 翻译说明：_notx 文件保留文件名；CONCURRENTLY / IF NOT EXISTS 删除（MySQL 的 CREATE INDEX
-- 不支持，迁移执行器保证单次执行）。

CREATE INDEX idx_account_groups_group_priority_account
    ON account_groups (group_id, priority, account_id);

CREATE INDEX idx_account_groups_account_priority_group
    ON account_groups (account_id, priority, group_id);
