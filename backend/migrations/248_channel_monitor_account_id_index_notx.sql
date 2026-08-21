CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_channel_monitors_account_id
    ON channel_monitors(account_id);
