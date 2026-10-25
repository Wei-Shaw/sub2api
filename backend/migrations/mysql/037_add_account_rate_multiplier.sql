-- MySQL 方言版本（翻译自 ../037_add_account_rate_multiplier.sql）

-- Add account billing rate multiplier and per-usage snapshot.
--
-- accounts.rate_multiplier: 账号计费倍率（>=0，允许 0 表示该账号计费为 0）。
-- usage_logs.account_rate_multiplier: 每条 usage log 的账号倍率快照，用于实现
--   “倍率调整仅影响之后请求”，并支持同一天分段倍率加权统计。
--
-- 注意：usage_logs.account_rate_multiplier 不做回填、不设置 NOT NULL。
-- 老数据为 NULL 时，统计口径按 1.0 处理（COALESCE）。

-- MySQL 不支持 ALTER TABLE IF EXISTS / ADD COLUMN IF NOT EXISTS，去掉修饰词
-- （迁移执行器保证每份迁移只执行一次）。
ALTER TABLE accounts
  ADD COLUMN rate_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0;

ALTER TABLE usage_logs
  ADD COLUMN account_rate_multiplier DECIMAL(10,4) NULL;
