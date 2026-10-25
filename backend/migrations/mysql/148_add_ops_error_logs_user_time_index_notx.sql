-- MySQL 方言版本（翻译自 ../148_add_ops_error_logs_user_time_index_notx.sql）
-- 翻译说明：_notx 文件保留文件名；CONCURRENTLY / IF NOT EXISTS 删除（MySQL 的 CREATE INDEX
-- 不支持，迁移执行器保证单次执行）。PG 部分索引 WHERE user_id IS NOT NULL（谓词仅含
-- IS NOT NULL）→ 普通索引（MySQL 唯一/普通索引天然允许多个 NULL）。

-- 148_add_ops_error_logs_user_time_index_notx.sql
-- 用户侧"错误请求"按 user_id 时间倒序分页所需的部分索引。
-- 非事务迁移（_notx）：CREATE INDEX CONCURRENTLY 不可在事务内执行。
CREATE INDEX idx_ops_error_logs_user_time
  ON ops_error_logs (user_id, created_at DESC);
