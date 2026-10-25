-- MySQL 方言版本（翻译自 ../010_add_usage_logs_aggregated_indexes.sql）
-- 为聚合查询补充复合索引
-- （CREATE INDEX IF NOT EXISTS → 去掉 IF NOT EXISTS，执行器保证单次执行）
CREATE INDEX idx_usage_logs_account_created_at ON usage_logs(account_id, created_at);
CREATE INDEX idx_usage_logs_api_key_created_at ON usage_logs(api_key_id, created_at);
CREATE INDEX idx_usage_logs_model_created_at ON usage_logs(model, created_at);
