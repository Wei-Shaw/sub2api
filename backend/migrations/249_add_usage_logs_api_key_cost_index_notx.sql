-- Serve per-key 30-day / today cost sums (API key list usage column) from the index alone,
-- instead of fetching hundreds of thousands of usage_logs heap rows per page load.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_api_key_created_cost
    ON usage_logs (api_key_id, created_at)
    INCLUDE (actual_cost);
