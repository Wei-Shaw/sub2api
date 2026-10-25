-- MySQL 方言版本（翻译自 ../124_backfill_legacy_oidc_security_flags.sql）
-- 翻译说明：
--   - WITH CTE 紧跟 INSERT ... SELECT（MySQL 8 允许 WITH 出现在 INSERT 的 SELECT 前）。
--   - VALUES 行构造器表 → UNION ALL 派生表；defaults(key) 别名列改为 defaults.`key`
--     （`key` 为 MySQL 保留字，加反引号）。
--   - WHERE NOT EXISTS 过滤 + ON CONFLICT (key) DO NOTHING → 保留 NOT EXISTS，
--     ON CONFLICT → ON DUPLICATE KEY UPDATE id = id。

INSERT INTO settings (`key`, value)
WITH legacy_oidc_install AS (
    SELECT 1
    FROM settings
    WHERE `key` IN (
        'oidc_connect_enabled',
        'oidc_connect_client_id',
        'oidc_connect_authorize_url',
        'oidc_connect_token_url',
        'oidc_connect_issuer_url',
        'oidc_connect_userinfo_url',
        'oidc_connect_frontend_redirect_url'
    )
    LIMIT 1
)
SELECT defaults.`key`, 'false'
FROM legacy_oidc_install
CROSS JOIN (
    SELECT 'oidc_connect_use_pkce' AS `key`
    UNION ALL
    SELECT 'oidc_connect_validate_id_token' AS `key`
) AS defaults
WHERE NOT EXISTS (
    SELECT 1
    FROM settings existing
    WHERE existing.`key` = defaults.`key`
)
ON DUPLICATE KEY UPDATE id = id;
