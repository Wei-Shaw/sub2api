-- 054_ops_system_logs.sql
-- 统一日志索引表与清理审计表

CREATE TABLE IF NOT EXISTS ops_system_logs (
  id BIGSERIAL PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  level VARCHAR(16) NOT NULL,
  component VARCHAR(128) NOT NULL DEFAULT '',
  message TEXT NOT NULL,
  request_id VARCHAR(128),
  client_request_id VARCHAR(128),
  user_id BIGINT,
  account_id BIGINT,
  platform VARCHAR(32),
  model VARCHAR(128),
  extra JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_ops_system_logs_created_at_id
  ON ops_system_logs (created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_ops_system_logs_level_created_at
  ON ops_system_logs (level, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_ops_system_logs_component_created_at
  ON ops_system_logs (component, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_ops_system_logs_request_id
  ON ops_system_logs (request_id);

CREATE INDEX IF NOT EXISTS idx_ops_system_logs_client_request_id
  ON ops_system_logs (client_request_id);

CREATE INDEX IF NOT EXISTS idx_ops_system_logs_user_id_created_at
  ON ops_system_logs (user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_ops_system_logs_account_id_created_at
  ON ops_system_logs (account_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_ops_system_logs_platform_model_created_at
  ON ops_system_logs (platform, model, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_ops_system_logs_message_search
  ON ops_system_logs USING GIN (to_tsvector('simple', COALESCE(message, '')));

CREATE TABLE IF NOT EXISTS ops_system_log_cleanup_audits (
  id BIGSERIAL PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  operator_id BIGINT NOT NULL,
  conditions JSONB NOT NULL DEFAULT '{}'::jsonb,
  deleted_rows BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_ops_system_log_cleanup_audits_created_at
  ON ops_system_log_cleanup_audits (created_at DESC, id DESC);
