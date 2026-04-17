-- Add account_cost column to dashboard aggregation tables for admin dashboard display.
-- account_cost = SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1))

ALTER TABLE usage_dashboard_hourly ADD COLUMN IF NOT EXISTS account_cost DECIMAL(20, 10) NOT NULL DEFAULT 0;
ALTER TABLE usage_dashboard_daily ADD COLUMN IF NOT EXISTS account_cost DECIMAL(20, 10) NOT NULL DEFAULT 0;
