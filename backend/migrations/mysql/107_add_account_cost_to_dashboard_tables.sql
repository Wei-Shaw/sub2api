-- MySQL 方言版本（翻译自 ../107_add_account_cost_to_dashboard_tables.sql）
-- 翻译说明：ADD COLUMN IF NOT EXISTS → ADD COLUMN（执行器保证单次执行）。

-- Add account_cost column to dashboard aggregation tables for admin dashboard display.
-- account_cost = SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1))
ALTER TABLE usage_dashboard_hourly ADD COLUMN account_cost DECIMAL(20, 10) NOT NULL DEFAULT 0;
ALTER TABLE usage_dashboard_daily ADD COLUMN account_cost DECIMAL(20, 10) NOT NULL DEFAULT 0;
