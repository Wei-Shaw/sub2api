-- Add the precomputed account-stats cost column on the host-owned
-- usage_logs table. Used by the channel-management plugin's
-- ResolveAccountStatsCost RPC: when the plugin returns a value, the
-- host stores it here so reporting queries can read
--   COALESCE(account_stats_cost, total_cost) * account_rate_multiplier
-- without re-running the plugin lookup for every aggregation.
--
-- usage_logs is host-owned; the plugin's SQL gate refuses to ALTER it,
-- so this companion migration ships on the host side. The plugin's
-- migration 013 adds the related channel-side tables.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS account_stats_cost NUMERIC(20,10);

COMMENT ON COLUMN usage_logs.account_stats_cost IS
    '账号统计自定义费用，NULL 表示走默认公式 (total_cost*account_rate_multiplier)';
