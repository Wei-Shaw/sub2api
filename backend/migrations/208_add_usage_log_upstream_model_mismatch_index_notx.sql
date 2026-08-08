CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_upstream_model_mismatch_created_at
    ON usage_logs (created_at DESC, id DESC)
    WHERE upstream_model_mismatch IS TRUE;
