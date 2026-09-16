-- user_platform_quotas 仅保存至少配置了一档限额的记录；不存在的行等价于不限额。
-- 三档限额全为 NULL 的行不携带任何可执行的限额，也不参与任何读取，直接删除。
-- 幂等：重复执行影响 0 行。
-- 行数约为历史用户数 × 平台数，整条语句在一个事务内执行；超大表需按
-- SETUP_MIGRATION_TIMEOUT_SECONDS 调高迁移超时，或在升级前预先分批清理。

DELETE FROM user_platform_quotas
 WHERE daily_limit_usd IS NULL
   AND weekly_limit_usd IS NULL
   AND monthly_limit_usd IS NULL;
