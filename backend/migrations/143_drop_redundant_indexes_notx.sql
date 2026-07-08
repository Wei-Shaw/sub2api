-- 143：删除被云数据库巡检标记为冗余的单列索引（均为既有复合/唯一索引的最左前缀，可被覆盖）。
-- 采用 *_notx + DROP INDEX CONCURRENTLY 避免在高写入表上持有排他锁；IF EXISTS 保证可重复执行。

DROP INDEX CONCURRENTLY IF EXISTS idx_usage_logs_user_id;
DROP INDEX CONCURRENTLY IF EXISTS idx_usage_logs_api_key_id;
DROP INDEX CONCURRENTLY IF EXISTS idx_usage_logs_account_id;
DROP INDEX CONCURRENTLY IF EXISTS idx_usage_logs_model;
DROP INDEX CONCURRENTLY IF EXISTS idx_usage_logs_subscription_id;
DROP INDEX CONCURRENTLY IF EXISTS idx_usage_logs_created_at;
DROP INDEX CONCURRENTLY IF EXISTS idx_usage_dashboard_hourly_users_bucket_start;
DROP INDEX CONCURRENTLY IF EXISTS idx_usage_dashboard_daily_users_bucket_date;
DROP INDEX CONCURRENTLY IF EXISTS idx_payment_audit_logs_order_id;
DROP INDEX CONCURRENTLY IF EXISTS idx_channel_monitors_provider;
DROP INDEX CONCURRENTLY IF EXISTS auth_identity_migration_reports_type_idx;
DROP INDEX CONCURRENTLY IF EXISTS auth_identities_user_id_idx;
DROP INDEX CONCURRENTLY IF EXISTS user_provider_default_grants_user_id_idx;
