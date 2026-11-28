CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_ingestion_cursor
    ON usage_logs (ingestion_txid, id);
