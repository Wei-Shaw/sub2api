-- Support upstream_model / mapping model distribution aggregations with time-range filters.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_created_model_upstream_model
ON usage_logs (created_at, model, upstream_model);
