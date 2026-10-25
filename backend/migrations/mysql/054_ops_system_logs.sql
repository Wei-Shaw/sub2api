-- MySQL 方言版本（翻译自 ../054_ops_system_logs.sql）
-- 统一日志索引表与清理审计表
--
-- 说明：PG 的 GIN(to_tsvector(...)) 全文表达式索引 → message 列的普通 B-tree 前缀索引
--       （TEXT 列必须指定前缀长度，191*4=764 字节 < InnoDB 3072 字节键长上限）。

CREATE TABLE IF NOT EXISTS ops_system_logs (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  level VARCHAR(16) NOT NULL,
  component VARCHAR(128) NOT NULL DEFAULT '',
  message TEXT NOT NULL,
  request_id VARCHAR(128),
  client_request_id VARCHAR(128),
  user_id BIGINT,
  account_id BIGINT,
  platform VARCHAR(32),
  model VARCHAR(128),
  extra JSON NOT NULL DEFAULT ('{}')
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_ops_system_logs_created_at_id
  ON ops_system_logs (created_at DESC, id DESC);

CREATE INDEX idx_ops_system_logs_level_created_at
  ON ops_system_logs (level, created_at DESC);

CREATE INDEX idx_ops_system_logs_component_created_at
  ON ops_system_logs (component, created_at DESC);

CREATE INDEX idx_ops_system_logs_request_id
  ON ops_system_logs (request_id);

CREATE INDEX idx_ops_system_logs_client_request_id
  ON ops_system_logs (client_request_id);

CREATE INDEX idx_ops_system_logs_user_id_created_at
  ON ops_system_logs (user_id, created_at DESC);

CREATE INDEX idx_ops_system_logs_account_id_created_at
  ON ops_system_logs (account_id, created_at DESC);

CREATE INDEX idx_ops_system_logs_platform_model_created_at
  ON ops_system_logs (platform, model, created_at DESC);

-- 原 GIN(to_tsvector('simple', COALESCE(message, ''))) → 普通前缀索引
CREATE INDEX idx_ops_system_logs_message_search
  ON ops_system_logs (message(191));

CREATE TABLE IF NOT EXISTS ops_system_log_cleanup_audits (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  operator_id BIGINT NOT NULL,
  conditions JSON NOT NULL DEFAULT ('{}'),
  deleted_rows BIGINT NOT NULL DEFAULT 0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_ops_system_log_cleanup_audits_created_at
  ON ops_system_log_cleanup_audits (created_at DESC, id DESC);
