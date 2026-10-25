-- MySQL 方言版本（翻译自 ../109_auth_identity_compat_backfill.sql）
--
-- 翻译说明：
--   - ON CONFLICT (provider_type, provider_key, provider_subject) DO NOTHING /
--     ON CONFLICT (report_type, report_key) DO NOTHING → ON DUPLICATE KEY UPDATE id = id
--     （对应 108 建立的唯一索引；MySQL 侧为 191 前缀唯一索引，见 108 头注）。
--   - BTRIM → TRIM；NOW() → NOW(3)；CAST(u.id AS TEXT) → CAST(u.id AS CHAR)。
--   - PG 正则捕获 SUBSTRING(x FROM '(?i)^linuxdo-(.+)@linuxdo-connect\.invalid$')
--     → 等价的「定长前后缀剥离」：SUBSTRING(x, LEN(prefix)+1, LEN(x)-LEN(prefix)-LEN(suffix))。
--     贪婪 (.+) 锚定串尾等价于从头部跳过前缀长度、从尾部剥掉后缀长度，且按长度截取与
--     大小写无关（无需 (?i)），不依赖 REGEXP_SUBSTR 的捕获组参数（8.0.17+ 才支持）。
--   - 正则匹配 ~ → REGEXP（输入先 LOWER，模式内 \. 按 MySQL 字符串字面量写作 \\.）。

INSERT INTO auth_identities (
    user_id,
    provider_type,
    provider_key,
    provider_subject,
    verified_at,
    metadata
)
SELECT
    u.id,
    'email',
    'email',
    LOWER(TRIM(u.email)),
    COALESCE(u.updated_at, u.created_at, NOW(3)),
    JSON_OBJECT(
        'backfill_source', 'users.email',
        'migration', '109_auth_identity_compat_backfill'
    )
FROM users AS u
WHERE u.deleted_at IS NULL
  AND TRIM(COALESCE(u.email, '')) <> ''
  AND RIGHT(LOWER(TRIM(u.email)), CHAR_LENGTH('@linuxdo-connect.invalid')) <> '@linuxdo-connect.invalid'
  AND RIGHT(LOWER(TRIM(u.email)), CHAR_LENGTH('@oidc-connect.invalid')) <> '@oidc-connect.invalid'
  AND RIGHT(LOWER(TRIM(u.email)), CHAR_LENGTH('@wechat-connect.invalid')) <> '@wechat-connect.invalid'
ON DUPLICATE KEY UPDATE id = auth_identities.id;

INSERT INTO auth_identities (
    user_id,
    provider_type,
    provider_key,
    provider_subject,
    verified_at,
    metadata
)
SELECT
    u.id,
    'linuxdo',
    'linuxdo',
    -- 等价于 SUBSTRING(x FROM '(?i)^linuxdo-(.+)@linuxdo-connect\.invalid$') 的捕获组
    SUBSTRING(
        TRIM(u.email),
        CHAR_LENGTH('linuxdo-') + 1,
        CHAR_LENGTH(TRIM(u.email)) - CHAR_LENGTH('linuxdo-') - CHAR_LENGTH('@linuxdo-connect.invalid')
    ),
    COALESCE(u.updated_at, u.created_at, NOW(3)),
    JSON_OBJECT(
        'backfill_source', 'synthetic_email',
        'legacy_email', TRIM(u.email),
        'migration', '109_auth_identity_compat_backfill'
    )
FROM users AS u
WHERE u.deleted_at IS NULL
  AND LOWER(TRIM(u.email)) REGEXP '^linuxdo-.+@linuxdo-connect\\.invalid$'
ON DUPLICATE KEY UPDATE id = auth_identities.id;

INSERT INTO auth_identities (
    user_id,
    provider_type,
    provider_key,
    provider_subject,
    verified_at,
    metadata
)
SELECT
    u.id,
    'wechat',
    'wechat',
    -- 等价于 SUBSTRING(x FROM '(?i)^wechat-(.+)@wechat-connect\.invalid$') 的捕获组
    SUBSTRING(
        TRIM(u.email),
        CHAR_LENGTH('wechat-') + 1,
        CHAR_LENGTH(TRIM(u.email)) - CHAR_LENGTH('wechat-') - CHAR_LENGTH('@wechat-connect.invalid')
    ),
    COALESCE(u.updated_at, u.created_at, NOW(3)),
    JSON_OBJECT(
        'backfill_source', 'synthetic_email',
        'legacy_email', TRIM(u.email),
        'migration', '109_auth_identity_compat_backfill'
    )
FROM users AS u
WHERE u.deleted_at IS NULL
  AND LOWER(TRIM(u.email)) REGEXP '^wechat-.+@wechat-connect\\.invalid$'
ON DUPLICATE KEY UPDATE id = auth_identities.id;

UPDATE users
SET signup_source = 'linuxdo'
WHERE deleted_at IS NULL
  AND LOWER(TRIM(COALESCE(email, ''))) REGEXP '^linuxdo-.+@linuxdo-connect\\.invalid$';

UPDATE users
SET signup_source = 'wechat'
WHERE deleted_at IS NULL
  AND LOWER(TRIM(COALESCE(email, ''))) REGEXP '^wechat-.+@wechat-connect\\.invalid$';

UPDATE users
SET signup_source = 'oidc'
WHERE deleted_at IS NULL
  AND LOWER(TRIM(COALESCE(email, ''))) REGEXP '^oidc-.+@oidc-connect\\.invalid$';

INSERT INTO auth_identity_migration_reports (report_type, report_key, details)
SELECT
    'oidc_synthetic_email_requires_manual_recovery',
    CAST(u.id AS CHAR),
    JSON_OBJECT(
        'user_id', u.id,
        'email', LOWER(TRIM(u.email)),
        'reason', 'cannot recover issuer_plus_sub deterministically from synthetic email alone',
        'migration', '109_auth_identity_compat_backfill'
    )
FROM users AS u
WHERE u.deleted_at IS NULL
  AND LOWER(TRIM(u.email)) REGEXP '^oidc-.+@oidc-connect\\.invalid$'
ON DUPLICATE KEY UPDATE id = auth_identity_migration_reports.id;

INSERT INTO auth_identity_migration_reports (report_type, report_key, details)
SELECT
    'wechat_openid_only_requires_remediation',
    CAST(u.id AS CHAR),
    JSON_OBJECT(
        'user_id', u.id,
        'email', LOWER(TRIM(u.email)),
        'reason', 'legacy wechat synthetic identity requires explicit unionid remediation if channel-only data exists',
        'migration', '109_auth_identity_compat_backfill'
    )
FROM users AS u
WHERE u.deleted_at IS NULL
  AND LOWER(TRIM(u.email)) REGEXP '^wechat-.+@wechat-connect\\.invalid$'
  AND NOT EXISTS (
      SELECT 1
      FROM auth_identities ai
      WHERE ai.user_id = u.id
        AND ai.provider_type = 'wechat'
        AND ai.provider_key = 'wechat'
  )
ON DUPLICATE KEY UPDATE id = auth_identity_migration_reports.id;
