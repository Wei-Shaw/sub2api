-- MySQL 方言版本（翻译自 ../155_add_ops_system_logs_api_key_id_index_notx.sql）
-- 翻译说明：_notx 文件保留文件名；CONCURRENTLY / IF NOT EXISTS 删除（MySQL 的 CREATE INDEX
-- 不支持，迁移执行器保证单次执行）。

-- 155_add_ops_system_logs_api_key_id_index_notx.sql
-- Non-transactional migration: CREATE INDEX CONCURRENTLY cannot run in a transaction.

CREATE INDEX idx_ops_system_logs_api_key_id_created_at
  ON ops_system_logs (api_key_id, created_at DESC);
