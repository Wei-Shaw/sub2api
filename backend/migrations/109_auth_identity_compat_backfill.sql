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
    LOWER(BTRIM(u.email)),
    COALESCE(u.updated_at, u.created_at, NOW()),
    jsonb_build_object(
        'backfill_source', 'users.email',
        'migration', '109_auth_identity_compat_backfill'
    )
FROM users AS u
WHERE u.deleted_at IS NULL
  AND BTRIM(COALESCE(u.email, '')) <> ''
  AND RIGHT(LOWER(BTRIM(u.email)), LENGTH('@linuxdo-connect.invalid')) <> '@linuxdo-connect.invalid'
  AND RIGHT(LOWER(BTRIM(u.email)), LENGTH('@oidc-connect.invalid')) <> '@oidc-connect.invalid'
  AND RIGHT(LOWER(BTRIM(u.email)), LENGTH('@wechat-connect.invalid')) <> '@wechat-connect.invalid'
ON CONFLICT (provider_type, provider_key, provider_subject) DO NOTHING;

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
    SUBSTRING(BTRIM(u.email) FROM '(?i)^linuxdo-(.+)@linuxdo-connect\.invalid$'),
    COALESCE(u.updated_at, u.created_at, NOW()),
    jsonb_build_object(
        'backfill_source', 'synthetic_email',
        'legacy_email', BTRIM(u.email),
        'migration', '109_auth_identity_compat_backfill'
    )
FROM users AS u
WHERE u.deleted_at IS NULL
  AND LOWER(BTRIM(u.email)) ~ '^linuxdo-.+@linuxdo-connect\.invalid$'
ON CONFLICT (provider_type, provider_key, provider_subject) DO NOTHING;

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
    SUBSTRING(BTRIM(u.email) FROM '(?i)^wechat-(.+)@wechat-connect\.invalid$'),
    COALESCE(u.updated_at, u.created_at, NOW()),
    jsonb_build_object(
        'backfill_source', 'synthetic_email',
        'legacy_email', BTRIM(u.email),
        'migration', '109_auth_identity_compat_backfill'
    )
FROM users AS u
WHERE u.deleted_at IS NULL
  AND LOWER(BTRIM(u.email)) ~ '^wechat-.+@wechat-connect\.invalid$'
ON CONFLICT (provider_type, provider_key, provider_subject) DO NOTHING;

UPDATE users
SET signup_source = 'linuxdo'
WHERE deleted_at IS NULL
  AND LOWER(BTRIM(COALESCE(email, ''))) ~ '^linuxdo-.+@linuxdo-connect\.invalid$';

UPDATE users
SET signup_source = 'wechat'
WHERE deleted_at IS NULL
  AND LOWER(BTRIM(COALESCE(email, ''))) ~ '^wechat-.+@wechat-connect\.invalid$';

UPDATE users
SET signup_source = 'oidc'
WHERE deleted_at IS NULL
  AND LOWER(BTRIM(COALESCE(email, ''))) ~ '^oidc-.+@oidc-connect\.invalid$';

INSERT INTO auth_identity_migration_reports (report_type, report_key, details)
SELECT
    'oidc_synthetic_email_requires_manual_recovery',
    CAST(u.id AS TEXT),
    jsonb_build_object(
        'user_id', u.id,
        'email', LOWER(BTRIM(u.email)),
        'reason', 'cannot recover issuer_plus_sub deterministically from synthetic email alone',
        'migration', '109_auth_identity_compat_backfill'
    )
FROM users AS u
WHERE u.deleted_at IS NULL
  AND LOWER(BTRIM(u.email)) ~ '^oidc-.+@oidc-connect\.invalid$'
ON CONFLICT (report_type, report_key) DO NOTHING;

INSERT INTO auth_identity_migration_reports (report_type, report_key, details)
SELECT
    'wechat_openid_only_requires_remediation',
    CAST(u.id AS TEXT),
    jsonb_build_object(
        'user_id', u.id,
        'email', LOWER(BTRIM(u.email)),
        'reason', 'legacy wechat synthetic identity requires explicit unionid remediation if channel-only data exists',
        'migration', '109_auth_identity_compat_backfill'
    )
FROM users AS u
WHERE u.deleted_at IS NULL
  AND LOWER(BTRIM(u.email)) ~ '^wechat-.+@wechat-connect\.invalid$'
  AND NOT EXISTS (
      SELECT 1
      FROM auth_identities ai
      WHERE ai.user_id = u.id
        AND ai.provider_type = 'wechat'
        AND ai.provider_key = 'wechat'
  )
ON CONFLICT (report_type, report_key) DO NOTHING;
