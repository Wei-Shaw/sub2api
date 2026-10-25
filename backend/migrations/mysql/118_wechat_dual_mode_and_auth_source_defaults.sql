-- MySQL 方言版本（翻译自 ../118_wechat_dual_mode_and_auth_source_defaults.sql）
-- 翻译说明：
--   - INSERT ... VALUES（CASE + 标量子查询）→ INSERT ... SELECT（派生表包裹 UNION ALL，
--     避免 UNION 与 ON DUPLICATE KEY UPDATE 的解析歧义）。settings 的 `key` 有唯一索引，
--     标量子查询至多返回一行，语义一致；`key` 为 MySQL 保留字，加反引号。
--   - 派生表在插入前物化，其中所有对 settings 的读取与 PG 单语句快照语义一致。
--   - ON CONFLICT (key) DO NOTHING → ON DUPLICATE KEY UPDATE id = id。

INSERT INTO settings (`key`, value)
SELECT rows_to_insert.`key`, rows_to_insert.value
FROM (
    SELECT 'wechat_connect_open_enabled' AS `key`,
           CASE
               WHEN NOT EXISTS (SELECT 1 FROM settings WHERE `key` = 'wechat_connect_enabled') THEN ''
               WHEN COALESCE((SELECT value FROM settings WHERE `key` = 'wechat_connect_enabled'), 'false') <> 'true' THEN 'false'
               WHEN LOWER(TRIM(COALESCE((SELECT value FROM settings WHERE `key` = 'wechat_connect_mode'), 'open'))) = 'mp' THEN 'false'
               ELSE 'true'
           END AS value
    UNION ALL
    SELECT 'wechat_connect_mp_enabled' AS `key`,
           CASE
               WHEN NOT EXISTS (SELECT 1 FROM settings WHERE `key` = 'wechat_connect_enabled') THEN ''
               WHEN COALESCE((SELECT value FROM settings WHERE `key` = 'wechat_connect_enabled'), 'false') <> 'true' THEN 'false'
               WHEN LOWER(TRIM(COALESCE((SELECT value FROM settings WHERE `key` = 'wechat_connect_mode'), 'open'))) = 'mp' THEN 'true'
               ELSE 'false'
           END AS value
    UNION ALL
    SELECT 'auth_source_default_email_grant_on_signup' AS `key`, 'false' AS value
    UNION ALL
    SELECT 'auth_source_default_linuxdo_grant_on_signup' AS `key`, 'false' AS value
    UNION ALL
    SELECT 'auth_source_default_oidc_grant_on_signup' AS `key`, 'false' AS value
    UNION ALL
    SELECT 'auth_source_default_wechat_grant_on_signup' AS `key`, 'false' AS value
) AS rows_to_insert
ON DUPLICATE KEY UPDATE id = id;
