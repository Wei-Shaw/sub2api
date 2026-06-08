-- Drop per-user daily quota schema (feature #1750).
--
-- 背景：
-- #1750 引入的 per-user daily quota 已被 service quota 替代，本次彻底清理
-- ent schema / code / 前端入口，所以需要把数据库侧的冗余也一并下线：
--   - users.usage_limit_enabled / users.daily_usage_limit_usd
--   - user_usage_limit_rules 表
--   - settings: usage_limit_enabled / default_usage_limit_enabled /
--     default_daily_usage_limit_usd
--
-- 幂等性：
-- 正式 / OpenAI / Star 镜像从未运行过 migration 101/102（不含这些 schema），
-- IF EXISTS 保证 no-op；Beta 通过 73ba3595 手工 ADD 回了列和表，这条 migration
-- 会把它们干净卸掉。

DROP TABLE IF EXISTS user_usage_limit_rules;

ALTER TABLE users DROP COLUMN IF EXISTS usage_limit_enabled;
ALTER TABLE users DROP COLUMN IF EXISTS daily_usage_limit_usd;

DELETE FROM settings
WHERE key IN (
    'usage_limit_enabled',
    'default_usage_limit_enabled',
    'default_daily_usage_limit_usd'
);
