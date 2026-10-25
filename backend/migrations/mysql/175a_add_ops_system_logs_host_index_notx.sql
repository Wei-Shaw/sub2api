-- MySQL 方言版本（翻译自 ../175a_add_ops_system_logs_host_index_notx.sql）
-- 翻译说明：_notx 文件保留文件名；CONCURRENTLY / IF NOT EXISTS 删除（MySQL 的 CREATE INDEX
-- 不支持，迁移执行器保证单次执行）。

CREATE INDEX idx_ops_system_logs_host_created_at
  ON ops_system_logs (host, created_at DESC);
