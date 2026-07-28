-- Indexes on pre-existing operational tables are built without blocking
-- normal writes during a production upgrade.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ops_metrics_hourly_v2_bucket
    ON ops_metrics_hourly (bucket_start DESC)
    WHERE metric_definition_version = 2;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ops_metrics_daily_v2_bucket
    ON ops_metrics_daily (bucket_date DESC)
    WHERE metric_definition_version = 2;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ops_error_logs_sla_outcome_time
    ON ops_error_logs (counts_toward_sla, final_outcome, created_at DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ops_error_logs_alert_family_time
    ON ops_error_logs (alert_family, created_at DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ops_error_logs_category_time
    ON ops_error_logs (error_category, created_at DESC);
