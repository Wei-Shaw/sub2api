CREATE OR REPLACE FUNCTION public.__migration_115_safe_legacy_metadata_jsonb(input_text TEXT)
RETURNS JSONB
LANGUAGE plpgsql
AS $$
DECLARE
    parsed JSONB;
BEGIN
    IF input_text IS NULL OR BTRIM(input_text) = '' THEN
        RETURN '{}'::jsonb;
    END IF;

    BEGIN
        parsed := input_text::jsonb;
    EXCEPTION
        WHEN OTHERS THEN
            RETURN '{}'::jsonb;
    END;

    IF jsonb_typeof(parsed) = 'object' THEN
        RETURN parsed;
    END IF;

    RETURN jsonb_build_object('_legacy_metadata_raw_json', parsed);
END;
$$;

DO $$
BEGIN
    IF to_regclass('public.user_external_identities') IS NULL THEN
        RETURN;
    END IF;

    EXECUTE $sql$
WITH legacy AS (
    SELECT
        uei.id,
        uei.user_id,
        BTRIM(uei.provider_user_id) AS provider_user_id,
        BTRIM(uei.provider_username) AS provider_username,
        BTRIM(uei.display_name) AS display_name,
        public.__migration_115_safe_legacy_metadata_jsonb(uei.metadata) AS metadata_json,
        uei.created_at,
        uei.updated_at
    FROM user_external_identities AS uei
    JOIN users AS u ON u.id = uei.user_id
    WHERE u.deleted_at IS NULL
      AND LOWER(BTRIM(COALESCE(uei.provider, ''))) = 'linuxdo'
      AND BTRIM(COALESCE(uei.provider_user_id, '')) <> ''
),
legacy_subjects AS (
    SELECT
        provider_user_id AS provider_subject,
        COUNT(DISTINCT user_id) AS distinct_user_count
    FROM legacy
    GROUP BY provider_user_id
),
canonical_legacy AS (
    SELECT
        legacy.*,
        ROW_NUMBER() OVER (
            PARTITION BY legacy.provider_user_id
            ORDER BY COALESCE(legacy.updated_at, legacy.created_at, NOW()) DESC, legacy.id DESC
        ) AS canonical_row_num
    FROM legacy
    JOIN legacy_subjects AS subjects
      ON subjects.provider_subject = legacy.provider_user_id
     AND subjects.distinct_user_count = 1
)
INSERT INTO auth_identities (
    user_id,
    provider_type,
    provider_key,
    provider_subject,
    verified_at,
    metadata
)
SELECT
    legacy.user_id,
    'linuxdo',
    'linuxdo',
    legacy.provider_user_id,
    COALESCE(legacy.updated_at, legacy.created_at, NOW()),
    legacy.metadata_json || jsonb_build_object(
        'legacy_identity_id', legacy.id,
        'provider_user_id', legacy.provider_user_id,
        'provider_username', legacy.provider_username,
        'display_name', legacy.display_name,
        'migration', '115_auth_identity_legacy_external_backfill'
    )
FROM canonical_legacy AS legacy
WHERE legacy.canonical_row_num = 1
ON CONFLICT (provider_type, provider_key, provider_subject) DO NOTHING;
$sql$;

    EXECUTE $sql$
WITH legacy AS (
    SELECT
        uei.id,
        uei.user_id,
        BTRIM(uei.provider_user_id) AS provider_user_id,
        BTRIM(uei.provider_union_id) AS provider_union_id,
        BTRIM(uei.provider_username) AS provider_username,
        BTRIM(uei.display_name) AS display_name,
        public.__migration_115_safe_legacy_metadata_jsonb(uei.metadata) AS metadata_json,
        uei.created_at,
        uei.updated_at
    FROM user_external_identities AS uei
    JOIN users AS u ON u.id = uei.user_id
    WHERE u.deleted_at IS NULL
      AND LOWER(BTRIM(COALESCE(uei.provider, ''))) = 'wechat'
      AND BTRIM(COALESCE(uei.provider_union_id, '')) <> ''
),
legacy_subjects AS (
    SELECT
        provider_union_id AS provider_subject,
        COUNT(DISTINCT user_id) AS distinct_user_count
    FROM legacy
    GROUP BY provider_union_id
),
canonical_legacy AS (
    SELECT
        legacy.*,
        ROW_NUMBER() OVER (
            PARTITION BY legacy.provider_union_id
            ORDER BY COALESCE(legacy.updated_at, legacy.created_at, NOW()) DESC, legacy.id DESC
        ) AS canonical_row_num
    FROM legacy
    JOIN legacy_subjects AS subjects
      ON subjects.provider_subject = legacy.provider_union_id
     AND subjects.distinct_user_count = 1
)
INSERT INTO auth_identities (
    user_id,
    provider_type,
    provider_key,
    provider_subject,
    verified_at,
    metadata
)
SELECT
    legacy.user_id,
    'wechat',
    'wechat-main',
    legacy.provider_union_id,
    COALESCE(legacy.updated_at, legacy.created_at, NOW()),
    legacy.metadata_json || jsonb_build_object(
        'legacy_identity_id', legacy.id,
        'openid', legacy.provider_user_id,
        'unionid', legacy.provider_union_id,
        'provider_user_id', legacy.provider_user_id,
        'provider_union_id', legacy.provider_union_id,
        'provider_username', legacy.provider_username,
        'display_name', legacy.display_name,
        'migration', '115_auth_identity_legacy_external_backfill'
    )
FROM canonical_legacy AS legacy
WHERE legacy.canonical_row_num = 1
ON CONFLICT (provider_type, provider_key, provider_subject) DO NOTHING;
$sql$;

    EXECUTE $sql$
WITH legacy AS (
    SELECT
        uei.user_id,
        BTRIM(uei.provider_user_id) AS provider_user_id,
        BTRIM(uei.provider_union_id) AS provider_union_id,
        BTRIM(COALESCE(meta.metadata_json ->> 'channel', '')) AS channel,
        BTRIM(COALESCE(meta.metadata_json ->> 'channel_app_id', meta.metadata_json ->> 'appid', meta.metadata_json ->> 'app_id', '')) AS channel_app_id,
        meta.metadata_json
    FROM user_external_identities AS uei
    JOIN users AS u ON u.id = uei.user_id
    CROSS JOIN LATERAL (
        SELECT public.__migration_115_safe_legacy_metadata_jsonb(uei.metadata) AS metadata_json
    ) AS meta
    WHERE u.deleted_at IS NULL
      AND LOWER(BTRIM(COALESCE(uei.provider, ''))) = 'wechat'
      AND BTRIM(COALESCE(uei.provider_union_id, '')) <> ''
),
legacy_subjects AS (
    SELECT
        provider_union_id AS provider_subject,
        COUNT(DISTINCT user_id) AS distinct_user_count
    FROM legacy
    GROUP BY provider_union_id
)
INSERT INTO auth_identity_channels (
    identity_id,
    provider_type,
    provider_key,
    channel,
    channel_app_id,
    channel_subject,
    metadata
)
SELECT
    ai.id,
    'wechat',
    'wechat-main',
    legacy.channel,
    legacy.channel_app_id,
    legacy.provider_user_id,
    legacy.metadata_json || jsonb_build_object(
        'openid', legacy.provider_user_id,
        'unionid', legacy.provider_union_id,
        'migration', '115_auth_identity_legacy_external_backfill'
    )
FROM legacy
JOIN legacy_subjects AS subjects
  ON subjects.provider_subject = legacy.provider_union_id
 AND subjects.distinct_user_count = 1
JOIN auth_identities AS ai
  ON ai.user_id = legacy.user_id
 AND ai.provider_type = 'wechat'
 AND ai.provider_key = 'wechat-main'
 AND ai.provider_subject = legacy.provider_union_id
WHERE legacy.channel <> ''
  AND legacy.channel_app_id <> ''
  AND legacy.provider_user_id <> ''
ON CONFLICT DO NOTHING;
$sql$;

    EXECUTE $sql$
INSERT INTO auth_identity_migration_reports (report_type, report_key, details)
SELECT
    'wechat_openid_only_requires_remediation',
    'legacy_external_identity:' || legacy.id::text,
    legacy.metadata_json || jsonb_build_object(
        'legacy_identity_id', legacy.id,
        'user_id', legacy.user_id,
        'openid', legacy.provider_user_id,
        'reason', 'legacy user_external_identities row only has openid and cannot be canonicalized offline',
        'migration', '115_auth_identity_legacy_external_backfill'
    )
FROM (
    SELECT
        uei.id,
        uei.user_id,
        BTRIM(uei.provider_user_id) AS provider_user_id,
        public.__migration_115_safe_legacy_metadata_jsonb(uei.metadata) AS metadata_json
    FROM user_external_identities AS uei
    JOIN users AS u ON u.id = uei.user_id
    WHERE u.deleted_at IS NULL
      AND LOWER(BTRIM(COALESCE(uei.provider, ''))) = 'wechat'
      AND BTRIM(COALESCE(uei.provider_user_id, '')) <> ''
      AND BTRIM(COALESCE(uei.provider_union_id, '')) = ''
) AS legacy
ON CONFLICT (report_type, report_key) DO NOTHING;
$sql$;
END $$;

INSERT INTO auth_identity_migration_reports (report_type, report_key, details)
SELECT
    'wechat_openid_only_requires_remediation',
    'synthetic_auth_identity:' || ai.id::text,
    COALESCE(ai.metadata, '{}'::jsonb) || jsonb_build_object(
        'auth_identity_id', ai.id,
        'user_id', ai.user_id,
        'provider_subject', ai.provider_subject,
        'reason', 'synthetic wechat auth identity still lacks unionid metadata and needs remediation',
        'migration', '115_auth_identity_legacy_external_backfill'
    )
FROM auth_identities AS ai
WHERE ai.provider_type = 'wechat'
  AND COALESCE(ai.metadata ->> 'backfill_source', '') = 'synthetic_email'
  AND BTRIM(COALESCE(ai.metadata ->> 'unionid', '')) = ''
ON CONFLICT (report_type, report_key) DO NOTHING;

DROP FUNCTION IF EXISTS public.__migration_115_safe_legacy_metadata_jsonb(TEXT);
