-- MySQL 方言版本（翻译自 ../078_add_usage_log_requested_model_index_notx.sql）
-- Support requested_model / upstream_model aggregations with time-range filters.
-- （文件名保留 _notx；MySQL 无 CONCURRENTLY/IF NOT EXISTS，由迁移执行器保证单次执行。）

CREATE INDEX idx_usage_logs_created_requested_model_upstream_model
ON usage_logs (created_at, requested_model, upstream_model);
