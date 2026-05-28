-- Add cache_policy_trace to usage_logs to record the cache policy decision taken per request.
-- Values: "acct_override:5m", "acct_override:1h", "global_inject:5m",
--         "eligible:no_override", "skip:not_supported".
-- NULL means the field was added after the row was written (historical rows).
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS cache_policy_trace VARCHAR(64) NULL DEFAULT NULL;
