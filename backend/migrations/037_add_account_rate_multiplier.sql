-- Add account billing rate multiplier and per-usage snapshot.
--
-- accounts.rate_multiplier: 账号计费倍率（>=0，允许 0 表示该账号计费为 0）。
-- usage_logs.account_rate_multiplier: 每条 usage log 的账号倍率快照，用于实现
--   “倍率调整仅影响之后请求”，并支持同一天分段倍率加权统计。
--
-- 注意：usage_logs.account_rate_multiplier 不做回填、不设置 NOT NULL。
-- 老数据为 NULL 时，统计口径按 1.0 处理（COALESCE）。

ALTER TABLE IF EXISTS accounts
  ADD COLUMN IF NOT EXISTS rate_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0;

ALTER TABLE IF EXISTS usage_logs
  ADD COLUMN IF NOT EXISTS account_rate_multiplier DECIMAL(10,4);
