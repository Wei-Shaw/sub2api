CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_account_id_id
    ON usage_logs (account_id, id);
