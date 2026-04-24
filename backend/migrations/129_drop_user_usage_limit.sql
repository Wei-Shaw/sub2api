-- Rollback of migration 101_add_user_usage_limit.sql + 102_normalize_zero_usage_limit.sql.
--
-- per-user daily quota (feature issue #1750) 已停用，统一由 service quota 规则（128）
-- 承担配额职责。生产环境里 #1750 从未真正上线（仅 Beta 应用过 101/102），
-- 且 Beta 上数据为 0 行 / 0 规则 / 开关=false，drop 安全无业务影响。
-- 代码侧（BillingCacheService.checkQuotaEligibility / QuotaService）暂保留为 dead
-- code，后续批次单独清理。

BEGIN;

DROP TABLE IF EXISTS user_usage_limit_rules;

ALTER TABLE users
    DROP COLUMN IF EXISTS usage_limit_enabled,
    DROP COLUMN IF EXISTS daily_usage_limit_usd;

DELETE FROM settings WHERE key = 'usage_limit_enabled';
DELETE FROM settings WHERE key = 'default_usage_limit_enabled';
DELETE FROM settings WHERE key = 'default_daily_usage_limit_usd';

COMMIT;
