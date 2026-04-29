CREATE OR REPLACE FUNCTION public.__migration_116_safe_legacy_metadata_jsonb(input_text TEXT)
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

CREATE OR REPLACE FUNCTION public.__migration_116_is_valid_legacy_metadata_jsonb(input_text TEXT)
RETURNS BOOLEAN
LANGUAGE plpgsql
AS $$
DECLARE
    parsed JSONB;
BEGIN
    IF input_text IS NULL OR BTRIM(input_text) = '' THEN
        RETURN TRUE;
    END IF;

    parsed := input_text::jsonb;
    RETURN TRUE;
EXCEPTION
    WHEN OTHERS THEN
        RETURN FALSE;
END;
$$;

DO $$
BEGIN
    IF to_regclass('public.user_external_identities') IS NULL THEN
        RETURN;
    END IF;

    EXECUTE $sql$
INSERT INTO auth_identity_migration_reports (report_type, report_key, details)
SELECT
    'legacy_external_identity_invalid_metadata_json',
    'legacy_external_identity:' || uei.id::text,
    jsonb_build_object(
        'legacy_identity_id', uei.id,
        'user_id', uei.user_id,
        'provider', LOWER(BTRIM(COALESCE(uei.provider, ''))),
        'provider_user_id', BTRIM(COALESCE(uei.provider_user_id, '')),
        'provider_union_id', BTRIM(COALESCE(uei.provider_union_id, '')),
        'reason', 'legacy metadata is not valid JSON; migration downgraded metadata to empty object',
        'raw_metadata', LEFT(BTRIM(COALESCE(uei.metadata, '')), 1000),
        'migration', '116_auth_identity_legacy_external_safety_reports'
    )
FROM user_external_identities AS uei
JOIN users AS u ON u.id = uei.user_id
WHERE u.deleted_at IS NULL
  AND BTRIM(COALESCE(uei.metadata, '')) <> ''
  AND NOT public.__migration_116_is_valid_legacy_metadata_jsonb(uei.metadata)
ON CONFLICT (report_type, report_key) DO NOTHING;
$sql$;

    EXECUTE $sql$
INSERT INTO auth_identity_migration_reports (report_type, report_key, details)
SELECT
    'legacy_external_identity_conflict',
    'legacy_external_identity:' || legacy.id::text,
    legacy.metadata_json || jsonb_build_object(
        'legacy_identity_id', legacy.id,
        'legacy_user_id', legacy.user_id,
        'provider_type', legacy.provider_type,
        'provider_key', legacy.provider_key,
        'provider_subject', legacy.provider_subject,
        'conflicting_legacy_user_ids', ambiguous.conflicting_legacy_user_ids,
        'reason', 'legacy canonical identity subject belongs to multiple legacy users and cannot be auto-resolved',
        'migration', '116_auth_identity_legacy_external_safety_reports'
    )
FROM (
    SELECT
        uei.id,
        uei.user_id,
        LOWER(BTRIM(COALESCE(uei.provider, ''))) AS provider_type,
        CASE
            WHEN LOWER(BTRIM(COALESCE(uei.provider, ''))) = 'wechat' THEN 'wechat-main'
            ELSE 'linuxdo'
        END AS provider_key,
        CASE
            WHEN LOWER(BTRIM(COALESCE(uei.provider, ''))) = 'wechat' THEN BTRIM(COALESCE(uei.provider_union_id, ''))
            ELSE BTRIM(COALESCE(uei.provider_user_id, ''))
        END AS provider_subject,
        public.__migration_116_safe_legacy_metadata_jsonb(uei.metadata) AS metadata_json
    FROM user_external_identities AS uei
    JOIN users AS u ON u.id = uei.user_id
    WHERE u.deleted_at IS NULL
      AND LOWER(BTRIM(COALESCE(uei.provider, ''))) IN ('linuxdo', 'wechat')
      AND (
          (LOWER(BTRIM(COALESCE(uei.provider, ''))) = 'linuxdo' AND BTRIM(COALESCE(uei.provider_user_id, '')) <> '')
          OR
          (LOWER(BTRIM(COALESCE(uei.provider, ''))) = 'wechat' AND BTRIM(COALESCE(uei.provider_union_id, '')) <> '')
      )
) AS legacy
JOIN (
    SELECT
        provider_type,
        provider_key,
        provider_subject,
        to_jsonb(array_agg(DISTINCT user_id ORDER BY user_id)) AS conflicting_legacy_user_ids
    FROM (
        SELECT
            uei.user_id,
            LOWER(BTRIM(COALESCE(uei.provider, ''))) AS provider_type,
            CASE
                WHEN LOWER(BTRIM(COALESCE(uei.provider, ''))) = 'wechat' THEN 'wechat-main'
                ELSE 'linuxdo'
            END AS provider_key,
            CASE
                WHEN LOWER(BTRIM(COALESCE(uei.provider, ''))) = 'wechat' THEN BTRIM(COALESCE(uei.provider_union_id, ''))
                ELSE BTRIM(COALESCE(uei.provider_user_id, ''))
            END AS provider_subject
        FROM user_external_identities AS uei
        JOIN users AS u ON u.id = uei.user_id
        WHERE u.deleted_at IS NULL
          AND LOWER(BTRIM(COALESCE(uei.provider, ''))) IN ('linuxdo', 'wechat')
          AND (
              (LOWER(BTRIM(COALESCE(uei.provider, ''))) = 'linuxdo' AND BTRIM(COALESCE(uei.provider_user_id, '')) <> '')
              OR
              (LOWER(BTRIM(COALESCE(uei.provider, ''))) = 'wechat' AND BTRIM(COALESCE(uei.provider_union_id, '')) <> '')
          )
    ) AS legacy_subjects
    GROUP BY provider_type, provider_key, provider_subject
    HAVING COUNT(DISTINCT user_id) > 1
) AS ambiguous
  ON ambiguous.provider_type = legacy.provider_type
 AND ambiguous.provider_key = legacy.provider_key
 AND ambiguous.provider_subject = legacy.provider_subject
ON CONFLICT (report_type, report_key) DO NOTHING;
$sql$;

    EXECUTE $sql$
INSERT INTO auth_identity_migration_reports (report_type, report_key, details)
SELECT
    'legacy_external_identity_conflict',
    'legacy_external_identity:' || legacy.id::text,
    legacy.metadata_json || jsonb_build_object(
        'legacy_identity_id', legacy.id,
        'legacy_user_id', legacy.user_id,
        'existing_identity_id', ai.id,
        'existing_user_id', ai.user_id,
        'provider_type', legacy.provider_type,
        'provider_key', legacy.provider_key,
        'provider_subject', legacy.provider_subject,
        'reason', 'legacy canonical identity subject already belongs to another user',
        'migration', '116_auth_identity_legacy_external_safety_reports'
    )
FROM (
    SELECT
        uei.id,
        uei.user_id,
        LOWER(BTRIM(COALESCE(uei.provider, ''))) AS provider_type,
        CASE
            WHEN LOWER(BTRIM(COALESCE(uei.provider, ''))) = 'wechat' THEN 'wechat-main'
            ELSE 'linuxdo'
        END AS provider_key,
        CASE
            WHEN LOWER(BTRIM(COALESCE(uei.provider, ''))) = 'wechat' THEN BTRIM(COALESCE(uei.provider_union_id, ''))
            ELSE BTRIM(COALESCE(uei.provider_user_id, ''))
        END AS provider_subject,
        BTRIM(COALESCE(uei.provider_user_id, '')) AS provider_user_id,
        BTRIM(COALESCE(uei.provider_union_id, '')) AS provider_union_id,
        BTRIM(COALESCE(uei.provider_username, '')) AS provider_username,
        BTRIM(COALESCE(uei.display_name, '')) AS display_name,
        public.__migration_116_safe_legacy_metadata_jsonb(uei.metadata) AS metadata_json
    FROM user_external_identities AS uei
    JOIN users AS u ON u.id = uei.user_id
    WHERE u.deleted_at IS NULL
      AND LOWER(BTRIM(COALESCE(uei.provider, ''))) IN ('linuxdo', 'wechat')
      AND (
          (LOWER(BTRIM(COALESCE(uei.provider, ''))) = 'linuxdo' AND BTRIM(COALESCE(uei.provider_user_id, '')) <> '')
          OR
          (LOWER(BTRIM(COALESCE(uei.provider, ''))) = 'wechat' AND BTRIM(COALESCE(uei.provider_union_id, '')) <> '')
      )
) AS legacy
JOIN (
    SELECT
        provider_type,
        provider_key,
        provider_subject
    FROM (
        SELECT
            uei.user_id,
            LOWER(BTRIM(COALESCE(uei.provider, ''))) AS provider_type,
            CASE
                WHEN LOWER(BTRIM(COALESCE(uei.provider, ''))) = 'wechat' THEN 'wechat-main'
                ELSE 'linuxdo'
            END AS provider_key,
            CASE
                WHEN LOWER(BTRIM(COALESCE(uei.provider, ''))) = 'wechat' THEN BTRIM(COALESCE(uei.provider_union_id, ''))
                ELSE BTRIM(COALESCE(uei.provider_user_id, ''))
            END AS provider_subject
        FROM user_external_identities AS uei
        JOIN users AS u ON u.id = uei.user_id
        WHERE u.deleted_at IS NULL
          AND LOWER(BTRIM(COALESCE(uei.provider, ''))) IN ('linuxdo', 'wechat')
          AND (
              (LOWER(BTRIM(COALESCE(uei.provider, ''))) = 'linuxdo' AND BTRIM(COALESCE(uei.provider_user_id, '')) <> '')
              OR
              (LOWER(BTRIM(COALESCE(uei.provider, ''))) = 'wechat' AND BTRIM(COALESCE(uei.provider_union_id, '')) <> '')
          )
    ) AS legacy_subjects
    GROUP BY provider_type, provider_key, provider_subject
    HAVING COUNT(DISTINCT user_id) = 1
) AS clear_subjects
  ON clear_subjects.provider_type = legacy.provider_type
 AND clear_subjects.provider_key = legacy.provider_key
 AND clear_subjects.provider_subject = legacy.provider_subject
JOIN auth_identities AS ai
  ON ai.provider_type = legacy.provider_type
 AND ai.provider_key = legacy.provider_key
 AND ai.provider_subject = legacy.provider_subject
WHERE ai.user_id <> legacy.user_id
ON CONFLICT (report_type, report_key) DO NOTHING;
$sql$;

    EXECUTE $sql$
WITH legacy AS (
    SELECT
        uei.id,
        uei.user_id,
        LOWER(BTRIM(COALESCE(uei.provider, ''))) AS provider_type,
        CASE
            WHEN LOWER(BTRIM(COALESCE(uei.provider, ''))) = 'wechat' THEN 'wechat-main'
            ELSE 'linuxdo'
        END AS provider_key,
        CASE
            WHEN LOWER(BTRIM(COALESCE(uei.provider, ''))) = 'wechat' THEN BTRIM(COALESCE(uei.provider_union_id, ''))
            ELSE BTRIM(COALESCE(uei.provider_user_id, ''))
        END AS provider_subject,
        BTRIM(COALESCE(uei.provider_user_id, '')) AS provider_user_id,
        BTRIM(COALESCE(uei.provider_union_id, '')) AS provider_union_id,
        BTRIM(COALESCE(uei.provider_username, '')) AS provider_username,
        BTRIM(COALESCE(uei.display_name, '')) AS display_name,
        public.__migration_116_safe_legacy_metadata_jsonb(uei.metadata) AS metadata_json,
        COALESCE(uei.updated_at, uei.created_at, NOW()) AS verified_at
    FROM user_external_identities AS uei
    JOIN users AS u ON u.id = uei.user_id
    WHERE u.deleted_at IS NULL
      AND LOWER(BTRIM(COALESCE(uei.provider, ''))) IN ('linuxdo', 'wechat')
      AND (
          (LOWER(BTRIM(COALESCE(uei.provider, ''))) = 'linuxdo' AND BTRIM(COALESCE(uei.provider_user_id, '')) <> '')
          OR
          (LOWER(BTRIM(COALESCE(uei.provider, ''))) = 'wechat' AND BTRIM(COALESCE(uei.provider_union_id, '')) <> '')
      )
),
clear_subjects AS (
    SELECT
        provider_type,
        provider_key,
        provider_subject
    FROM legacy
    GROUP BY provider_type, provider_key, provider_subject
    HAVING COUNT(DISTINCT user_id) = 1
),
canonical_legacy AS (
    SELECT
        legacy.*,
        ROW_NUMBER() OVER (
            PARTITION BY legacy.provider_type, legacy.provider_key, legacy.provider_subject
            ORDER BY legacy.verified_at DESC, legacy.id DESC
        ) AS canonical_row_num
    FROM legacy
    JOIN clear_subjects
      ON clear_subjects.provider_type = legacy.provider_type
     AND clear_subjects.provider_key = legacy.provider_key
     AND clear_subjects.provider_subject = legacy.provider_subject
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
    legacy.provider_type,
    legacy.provider_key,
    legacy.provider_subject,
    legacy.verified_at,
    legacy.metadata_json || jsonb_build_object(
        'legacy_identity_id', legacy.id,
        'provider_user_id', legacy.provider_user_id,
        'provider_union_id', NULLIF(legacy.provider_union_id, ''),
        'provider_username', legacy.provider_username,
        'display_name', legacy.display_name,
        'migration', '116_auth_identity_legacy_external_safety_reports'
    )
FROM canonical_legacy AS legacy
LEFT JOIN auth_identities AS ai
  ON ai.provider_type = legacy.provider_type
 AND ai.provider_key = legacy.provider_key
 AND ai.provider_subject = legacy.provider_subject
WHERE legacy.canonical_row_num = 1
  AND ai.id IS NULL
ON CONFLICT (provider_type, provider_key, provider_subject) DO NOTHING;
$sql$;

    EXECUTE $sql$
INSERT INTO auth_identity_migration_reports (report_type, report_key, details)
SELECT
    'legacy_external_channel_conflict',
    'legacy_external_identity:' || legacy.id::text,
    legacy.metadata_json || jsonb_build_object(
        'legacy_identity_id', legacy.id,
        'legacy_user_id', legacy.user_id,
        'existing_channel_id', channel.id,
        'existing_identity_id', existing_ai.id,
        'existing_user_id', existing_ai.user_id,
        'provider_type', 'wechat',
        'provider_key', 'wechat-main',
        'provider_subject', legacy.provider_union_id,
        'channel', legacy.channel,
        'channel_app_id', legacy.channel_app_id,
        'channel_subject', legacy.provider_user_id,
        'reason', 'legacy channel subject already belongs to another user',
        'migration', '116_auth_identity_legacy_external_safety_reports'
    )
FROM (
    SELECT
        uei.id,
        uei.user_id,
        BTRIM(COALESCE(uei.provider_user_id, '')) AS provider_user_id,
        BTRIM(COALESCE(uei.provider_union_id, '')) AS provider_union_id,
        public.__migration_116_safe_legacy_metadata_jsonb(uei.metadata) AS metadata_json,
        BTRIM(COALESCE(public.__migration_116_safe_legacy_metadata_jsonb(uei.metadata) ->> 'channel', '')) AS channel,
        BTRIM(COALESCE(
            public.__migration_116_safe_legacy_metadata_jsonb(uei.metadata) ->> 'channel_app_id',
            public.__migration_116_safe_legacy_metadata_jsonb(uei.metadata) ->> 'appid',
            public.__migration_116_safe_legacy_metadata_jsonb(uei.metadata) ->> 'app_id',
            ''
        )) AS channel_app_id
    FROM user_external_identities AS uei
    JOIN users AS u ON u.id = uei.user_id
    WHERE u.deleted_at IS NULL
      AND LOWER(BTRIM(COALESCE(uei.provider, ''))) = 'wechat'
      AND BTRIM(COALESCE(uei.provider_union_id, '')) <> ''
      AND BTRIM(COALESCE(uei.provider_user_id, '')) <> ''
) AS legacy
JOIN (
    SELECT
        BTRIM(COALESCE(uei.provider_union_id, '')) AS provider_subject
    FROM user_external_identities AS uei
    JOIN users AS u ON u.id = uei.user_id
    WHERE u.deleted_at IS NULL
      AND LOWER(BTRIM(COALESCE(uei.provider, ''))) = 'wechat'
      AND BTRIM(COALESCE(uei.provider_union_id, '')) <> ''
      AND BTRIM(COALESCE(uei.provider_user_id, '')) <> ''
    GROUP BY BTRIM(COALESCE(uei.provider_union_id, ''))
    HAVING COUNT(DISTINCT uei.user_id) = 1
) AS clear_subjects
  ON clear_subjects.provider_subject = legacy.provider_union_id
JOIN auth_identities AS legacy_ai
  ON legacy_ai.user_id = legacy.user_id
 AND legacy_ai.provider_type = 'wechat'
 AND legacy_ai.provider_key = 'wechat-main'
 AND legacy_ai.provider_subject = legacy.provider_union_id
JOIN auth_identity_channels AS channel
  ON channel.provider_type = 'wechat'
 AND channel.provider_key = 'wechat-main'
 AND channel.channel = legacy.channel
 AND channel.channel_app_id = legacy.channel_app_id
 AND channel.channel_subject = legacy.provider_user_id
JOIN auth_identities AS existing_ai
  ON existing_ai.id = channel.identity_id
WHERE legacy.channel <> ''
  AND legacy.channel_app_id <> ''
  AND existing_ai.user_id <> legacy.user_id
ON CONFLICT (report_type, report_key) DO NOTHING;
$sql$;

    EXECUTE $sql$
WITH legacy AS (
    SELECT
        uei.user_id,
        BTRIM(COALESCE(uei.provider_user_id, '')) AS provider_user_id,
        BTRIM(COALESCE(uei.provider_union_id, '')) AS provider_union_id,
        public.__migration_116_safe_legacy_metadata_jsonb(uei.metadata) AS metadata_json,
        BTRIM(COALESCE(public.__migration_116_safe_legacy_metadata_jsonb(uei.metadata) ->> 'channel', '')) AS channel,
        BTRIM(COALESCE(
            public.__migration_116_safe_legacy_metadata_jsonb(uei.metadata) ->> 'channel_app_id',
            public.__migration_116_safe_legacy_metadata_jsonb(uei.metadata) ->> 'appid',
            public.__migration_116_safe_legacy_metadata_jsonb(uei.metadata) ->> 'app_id',
            ''
        )) AS channel_app_id
    FROM user_external_identities AS uei
    JOIN users AS u ON u.id = uei.user_id
    WHERE u.deleted_at IS NULL
      AND LOWER(BTRIM(COALESCE(uei.provider, ''))) = 'wechat'
      AND BTRIM(COALESCE(uei.provider_union_id, '')) <> ''
      AND BTRIM(COALESCE(uei.provider_user_id, '')) <> ''
),
clear_subjects AS (
    SELECT
        provider_union_id AS provider_subject
    FROM legacy
    GROUP BY provider_union_id
    HAVING COUNT(DISTINCT user_id) = 1
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
    legacy_ai.id,
    'wechat',
    'wechat-main',
    legacy.channel,
    legacy.channel_app_id,
    legacy.provider_user_id,
    legacy.metadata_json || jsonb_build_object(
        'openid', legacy.provider_user_id,
        'unionid', legacy.provider_union_id,
        'migration', '116_auth_identity_legacy_external_safety_reports'
    )
FROM legacy
JOIN clear_subjects
  ON clear_subjects.provider_subject = legacy.provider_union_id
JOIN auth_identities AS legacy_ai
  ON legacy_ai.user_id = legacy.user_id
 AND legacy_ai.provider_type = 'wechat'
 AND legacy_ai.provider_key = 'wechat-main'
 AND legacy_ai.provider_subject = legacy.provider_union_id
LEFT JOIN auth_identity_channels AS channel
  ON channel.provider_type = 'wechat'
 AND channel.provider_key = 'wechat-main'
 AND channel.channel = legacy.channel
 AND channel.channel_app_id = legacy.channel_app_id
 AND channel.channel_subject = legacy.provider_user_id
WHERE legacy.channel <> ''
  AND legacy.channel_app_id <> ''
  AND channel.id IS NULL
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
        'migration', '116_auth_identity_legacy_external_safety_reports'
    )
FROM (
    SELECT
        uei.id,
        uei.user_id,
        BTRIM(COALESCE(uei.provider_user_id, '')) AS provider_user_id,
        public.__migration_116_safe_legacy_metadata_jsonb(uei.metadata) AS metadata_json
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

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'auth_identities_metadata_is_object_check'
    ) THEN
        ALTER TABLE auth_identities
            ADD CONSTRAINT auth_identities_metadata_is_object_check
            CHECK (jsonb_typeof(metadata) = 'object');
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'auth_identity_channels_metadata_is_object_check'
    ) THEN
        ALTER TABLE auth_identity_channels
            ADD CONSTRAINT auth_identity_channels_metadata_is_object_check
            CHECK (jsonb_typeof(metadata) = 'object');
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'auth_identity_migration_reports_details_is_object_check'
    ) THEN
        ALTER TABLE auth_identity_migration_reports
            ADD CONSTRAINT auth_identity_migration_reports_details_is_object_check
            CHECK (jsonb_typeof(details) = 'object');
    END IF;
END $$;

DROP FUNCTION IF EXISTS public.__migration_116_is_valid_legacy_metadata_jsonb(TEXT);
DROP FUNCTION IF EXISTS public.__migration_116_safe_legacy_metadata_jsonb(TEXT);
