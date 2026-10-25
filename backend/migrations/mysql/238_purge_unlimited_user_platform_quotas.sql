-- MySQL 方言版本（翻译自 ../238_purge_unlimited_user_platform_quotas.sql）
-- 翻译说明：纯 DELETE 语句，语法等价，原样保留。

-- user_platform_quotas 仅保存至少配置了一档限额的记录；不存在的行等价于不限额。
-- 三档限额全为 NULL 的行不携带任何可执行的限额，也不参与任何读取，直接删除。
-- 幂等：重复执行影响 0 行。
DELETE FROM user_platform_quotas
 WHERE daily_limit_usd IS NULL
   AND weekly_limit_usd IS NULL
   AND monthly_limit_usd IS NULL;
