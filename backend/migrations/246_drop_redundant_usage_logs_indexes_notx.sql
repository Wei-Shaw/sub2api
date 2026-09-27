-- Drop single-column usage_logs indexes that production does not scan (pg_stat_user_indexes).
-- idx_usage_logs_subscription_id and idx_usage_logs_model are left prefixes of
-- idx_usage_logs_sub_created (003) and idx_usage_logs_model_created_at (010), which keep serving
-- equality lookups and the subscription_id foreign key (ON DELETE SET NULL).
-- idx_usage_logs_ip_address backs no query: the latest-IP lookup uses idx_usage_logs_api_key_latest_ip (174).
-- idx_usage_logs_user_id, idx_usage_logs_api_key_id and idx_usage_logs_account_id stay: the planner still uses them.
DROP INDEX CONCURRENTLY IF EXISTS idx_usage_logs_ip_address;
DROP INDEX CONCURRENTLY IF EXISTS idx_usage_logs_subscription_id;
DROP INDEX CONCURRENTLY IF EXISTS idx_usage_logs_model;
