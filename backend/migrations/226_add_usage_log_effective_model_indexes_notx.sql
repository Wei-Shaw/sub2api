CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_effective_requested_model_created
    ON usage_logs (
        (COALESCE(NULLIF(BTRIM(requested_model), ''), model)),
        created_at DESC,
        id DESC
    );

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_effective_upstream_model_created
    ON usage_logs (
        (COALESCE(NULLIF(BTRIM(upstream_model), ''), model)),
        created_at DESC,
        id DESC
    );
