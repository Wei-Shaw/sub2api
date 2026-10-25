-- MySQL 方言版本（翻译自 ../175_add_ops_system_logs_host.sql）
-- 翻译说明：ADD COLUMN IF NOT EXISTS → ADD COLUMN（IF NOT EXISTS 按迁移执行器单次执行语义去掉）。

-- Track the application host that emitted each indexed system log.
ALTER TABLE ops_system_logs
  ADD COLUMN host VARCHAR(255) NULL;
