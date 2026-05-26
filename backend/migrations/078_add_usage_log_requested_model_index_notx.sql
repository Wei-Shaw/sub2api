-- Support requested_model / upstream_model aggregations with time-range filters.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_created_requested_model_upstream_model
ON usage_logs (created_at, requested_model, upstream_model);
