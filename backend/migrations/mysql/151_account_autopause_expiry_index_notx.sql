-- MySQL 方言版本（翻译自 ../151_account_autopause_expiry_index_notx.sql）
-- 翻译说明：_notx 文件保留文件名；CONCURRENTLY / IF NOT EXISTS 删除（MySQL 的 CREATE INDEX
-- 不支持，迁移执行器保证单次执行）。PG 部分索引（谓词含 deleted_at IS NULL /
-- schedulable = TRUE / auto_pause_on_expired = TRUE / expires_at IS NOT NULL，非唯一）
-- → 按规则去掉 WHERE 的普通索引（仅性能差异）。

CREATE INDEX idx_accounts_autopause_expiry_due
    ON accounts (expires_at);
