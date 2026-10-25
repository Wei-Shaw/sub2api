-- MySQL 方言版本（翻译自 ../116_auth_identity_legacy_external_safety_reports.sql）
--
-- 翻译决策：
--   - 两个辅助函数（safe/valid legacy metadata）内联为 CASE / JSON_VALID 表达式，
--     语义与 PG 逐分支对齐：NULL/空白或非法 JSON → {}（is_valid 视为 TRUE）；
--     合法 JSON 且为 object → 原值；其余 → {"_legacy_metadata_raw_json": <原 JSON 值>}。
--   - DO 块的 user_external_identities 存在性守卫 → 临时存储过程，CALL 后即 DROP。
--   - jsonb || → JSON_MERGE_PATCH：右侧键值为 SQL NULL 时该键被移除（PG || 会写入 JSON null），
--     仅影响 116 第 4 条 INSERT 中 NULLIF(provider_union_id,'') 为空时的展示差异
--     （读取方 ->> / map 取值行为一致），无功能影响。
--   - to_jsonb(array_agg(DISTINCT user_id ORDER BY user_id)) → 先 SELECT DISTINCT + ORDER BY
--     再 JSON_ARRAYAGG 聚合（JSON_ARRAYAGG 不支持 DISTINCT/ORDER BY，元素顺序尽力保持，
--     仅影响报告 details 的展示顺序）。
--   - ON CONFLICT ... DO NOTHING → ON DUPLICATE KEY UPDATE id = id；BTRIM → TRIM；
--     jsonb_typeof(x) = 'object' → JSON_TYPE(x) = 'OBJECT'（用于 CHECK 约束）。

DROP PROCEDURE IF EXISTS __migration_116_tmp;
CREATE PROCEDURE __migration_116_tmp()
BEGIN
    -- 遗留表 user_external_identities 仅存在于历史库；全新安装直接跳过。
    IF EXISTS (
        SELECT 1
        FROM information_schema.tables
        WHERE table_schema = DATABASE()
          AND table_name = 'user_external_identities'
    ) THEN

        -- 1. 遗留 metadata 非法 JSON → 降级报告
        INSERT INTO auth_identity_migration_reports (report_type, report_key, details)
        SELECT
            'legacy_external_identity_invalid_metadata_json',
            CONCAT('legacy_external_identity:', uei.id),
            JSON_OBJECT(
                'legacy_identity_id', uei.id,
                'user_id', uei.user_id,
                'provider', LOWER(TRIM(COALESCE(uei.provider, ''))),
                'provider_user_id', TRIM(COALESCE(uei.provider_user_id, '')),
                'provider_union_id', TRIM(COALESCE(uei.provider_union_id, '')),
                'reason', 'legacy metadata is not valid JSON; migration downgraded metadata to empty object',
                'raw_metadata', LEFT(TRIM(COALESCE(uei.metadata, '')), 1000),
                'migration', '116_auth_identity_legacy_external_safety_reports'
            )
        FROM user_external_identities AS uei
        JOIN users AS u ON u.id = uei.user_id
        WHERE u.deleted_at IS NULL
          AND TRIM(COALESCE(uei.metadata, '')) <> ''
          AND NOT JSON_VALID(uei.metadata)
        ON DUPLICATE KEY UPDATE id = id;

        -- 2. 同一 canonical subject 归属多个遗留用户且无法自动归并 → 冲突报告
        INSERT INTO auth_identity_migration_reports (report_type, report_key, details)
        SELECT
            'legacy_external_identity_conflict',
            CONCAT('legacy_external_identity:', legacy.id),
            JSON_MERGE_PATCH(
                legacy.metadata_json,
                JSON_OBJECT(
                    'legacy_identity_id', legacy.id,
                    'legacy_user_id', legacy.user_id,
                    'provider_type', legacy.provider_type,
                    'provider_key', legacy.provider_key,
                    'provider_subject', legacy.provider_subject,
                    'conflicting_legacy_user_ids', ambiguous.conflicting_legacy_user_ids,
                    'reason', 'legacy canonical identity subject belongs to multiple legacy users and cannot be auto-resolved',
                    'migration', '116_auth_identity_legacy_external_safety_reports'
                )
            )
        FROM (
            SELECT
                uei.id,
                uei.user_id,
                LOWER(TRIM(COALESCE(uei.provider, ''))) AS provider_type,
                CASE
                    WHEN LOWER(TRIM(COALESCE(uei.provider, ''))) = 'wechat' THEN 'wechat-main'
                    ELSE 'linuxdo'
                END AS provider_key,
                CASE
                    WHEN LOWER(TRIM(COALESCE(uei.provider, ''))) = 'wechat' THEN TRIM(COALESCE(uei.provider_union_id, ''))
                    ELSE TRIM(COALESCE(uei.provider_user_id, ''))
                END AS provider_subject,
                CASE
                    WHEN uei.metadata IS NULL OR TRIM(uei.metadata) = '' THEN CAST('{}' AS JSON)
                    WHEN NOT JSON_VALID(uei.metadata) THEN CAST('{}' AS JSON)
                    WHEN JSON_TYPE(uei.metadata) = 'OBJECT' THEN CAST(uei.metadata AS JSON)
                    ELSE JSON_OBJECT('_legacy_metadata_raw_json', CAST(uei.metadata AS JSON))
                END AS metadata_json
            FROM user_external_identities AS uei
            JOIN users AS u ON u.id = uei.user_id
            WHERE u.deleted_at IS NULL
              AND LOWER(TRIM(COALESCE(uei.provider, ''))) IN ('linuxdo', 'wechat')
              AND (
                  (LOWER(TRIM(COALESCE(uei.provider, ''))) = 'linuxdo' AND TRIM(COALESCE(uei.provider_user_id, '')) <> '')
                  OR
                  (LOWER(TRIM(COALESCE(uei.provider, ''))) = 'wechat' AND TRIM(COALESCE(uei.provider_union_id, '')) <> '')
              )
        ) AS legacy
        JOIN (
            SELECT
                ordered.provider_type,
                ordered.provider_key,
                ordered.provider_subject,
                JSON_ARRAYAGG(ordered.user_id) AS conflicting_legacy_user_ids
            FROM (
                -- 近似 PG array_agg(DISTINCT user_id ORDER BY user_id) 的元素顺序
                SELECT user_id, provider_type, provider_key, provider_subject
                FROM (
                    SELECT DISTINCT
                        s.user_id,
                        LOWER(TRIM(COALESCE(s.provider, ''))) AS provider_type,
                        CASE
                            WHEN LOWER(TRIM(COALESCE(s.provider, ''))) = 'wechat' THEN 'wechat-main'
                            ELSE 'linuxdo'
                        END AS provider_key,
                        CASE
                            WHEN LOWER(TRIM(COALESCE(s.provider, ''))) = 'wechat' THEN TRIM(COALESCE(s.provider_union_id, ''))
                            ELSE TRIM(COALESCE(s.provider_user_id, ''))
                        END AS provider_subject
                    FROM user_external_identities AS s
                    JOIN users AS su ON su.id = s.user_id
                    WHERE su.deleted_at IS NULL
                      AND LOWER(TRIM(COALESCE(s.provider, ''))) IN ('linuxdo', 'wechat')
                      AND (
                          (LOWER(TRIM(COALESCE(s.provider, ''))) = 'linuxdo' AND TRIM(COALESCE(s.provider_user_id, '')) <> '')
                          OR
                          (LOWER(TRIM(COALESCE(s.provider, ''))) = 'wechat' AND TRIM(COALESCE(s.provider_union_id, '')) <> '')
                      )
                ) AS legacy_subjects
                ORDER BY legacy_subjects.user_id
            ) AS ordered
            GROUP BY ordered.provider_type, ordered.provider_key, ordered.provider_subject
            HAVING COUNT(DISTINCT ordered.user_id) > 1
        ) AS ambiguous
          ON ambiguous.provider_type = legacy.provider_type
         AND ambiguous.provider_key = legacy.provider_key
         AND ambiguous.provider_subject = legacy.provider_subject
        ON DUPLICATE KEY UPDATE id = id;

        -- 3. 唯一归属 subject 但 auth_identities 已被其他用户占用 → 冲突报告
        INSERT INTO auth_identity_migration_reports (report_type, report_key, details)
        SELECT
            'legacy_external_identity_conflict',
            CONCAT('legacy_external_identity:', legacy.id),
            JSON_MERGE_PATCH(
                legacy.metadata_json,
                JSON_OBJECT(
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
            )
        FROM (
            SELECT
                uei.id,
                uei.user_id,
                LOWER(TRIM(COALESCE(uei.provider, ''))) AS provider_type,
                CASE
                    WHEN LOWER(TRIM(COALESCE(uei.provider, ''))) = 'wechat' THEN 'wechat-main'
                    ELSE 'linuxdo'
                END AS provider_key,
                CASE
                    WHEN LOWER(TRIM(COALESCE(uei.provider, ''))) = 'wechat' THEN TRIM(COALESCE(uei.provider_union_id, ''))
                    ELSE TRIM(COALESCE(uei.provider_user_id, ''))
                END AS provider_subject,
                CASE
                    WHEN uei.metadata IS NULL OR TRIM(uei.metadata) = '' THEN CAST('{}' AS JSON)
                    WHEN NOT JSON_VALID(uei.metadata) THEN CAST('{}' AS JSON)
                    WHEN JSON_TYPE(uei.metadata) = 'OBJECT' THEN CAST(uei.metadata AS JSON)
                    ELSE JSON_OBJECT('_legacy_metadata_raw_json', CAST(uei.metadata AS JSON))
                END AS metadata_json
            FROM user_external_identities AS uei
            JOIN users AS u ON u.id = uei.user_id
            WHERE u.deleted_at IS NULL
              AND LOWER(TRIM(COALESCE(uei.provider, ''))) IN ('linuxdo', 'wechat')
              AND (
                  (LOWER(TRIM(COALESCE(uei.provider, ''))) = 'linuxdo' AND TRIM(COALESCE(uei.provider_user_id, '')) <> '')
                  OR
                  (LOWER(TRIM(COALESCE(uei.provider, ''))) = 'wechat' AND TRIM(COALESCE(uei.provider_union_id, '')) <> '')
              )
        ) AS legacy
        JOIN (
            SELECT
                legacy_subjects.provider_type,
                legacy_subjects.provider_key,
                legacy_subjects.provider_subject
            FROM (
                SELECT DISTINCT
                    s.user_id,
                    LOWER(TRIM(COALESCE(s.provider, ''))) AS provider_type,
                    CASE
                        WHEN LOWER(TRIM(COALESCE(s.provider, ''))) = 'wechat' THEN 'wechat-main'
                        ELSE 'linuxdo'
                    END AS provider_key,
                    CASE
                        WHEN LOWER(TRIM(COALESCE(s.provider, ''))) = 'wechat' THEN TRIM(COALESCE(s.provider_union_id, ''))
                        ELSE TRIM(COALESCE(s.provider_user_id, ''))
                    END AS provider_subject
                FROM user_external_identities AS s
                JOIN users AS su ON su.id = s.user_id
                WHERE su.deleted_at IS NULL
                  AND LOWER(TRIM(COALESCE(s.provider, ''))) IN ('linuxdo', 'wechat')
                  AND (
                      (LOWER(TRIM(COALESCE(s.provider, ''))) = 'linuxdo' AND TRIM(COALESCE(s.provider_user_id, '')) <> '')
                      OR
                      (LOWER(TRIM(COALESCE(s.provider, ''))) = 'wechat' AND TRIM(COALESCE(s.provider_union_id, '')) <> '')
                  )
            ) AS legacy_subjects
            GROUP BY legacy_subjects.provider_type, legacy_subjects.provider_key, legacy_subjects.provider_subject
            HAVING COUNT(DISTINCT legacy_subjects.user_id) = 1
        ) AS clear_subjects
          ON clear_subjects.provider_type = legacy.provider_type
         AND clear_subjects.provider_key = legacy.provider_key
         AND clear_subjects.provider_subject = legacy.provider_subject
        JOIN auth_identities AS ai
          ON ai.provider_type = legacy.provider_type
         AND ai.provider_key = legacy.provider_key
         AND ai.provider_subject = legacy.provider_subject
        WHERE ai.user_id <> legacy.user_id
        ON DUPLICATE KEY UPDATE id = id;

        -- 4. 唯一归属 subject 且未被占用 → 回填 auth_identities（每 subject 取最新一行）
        INSERT INTO auth_identities (
            user_id,
            provider_type,
            provider_key,
            provider_subject,
            verified_at,
            metadata
        )
        WITH legacy AS (
            SELECT
                uei.id,
                uei.user_id,
                LOWER(TRIM(COALESCE(uei.provider, ''))) AS provider_type,
                CASE
                    WHEN LOWER(TRIM(COALESCE(uei.provider, ''))) = 'wechat' THEN 'wechat-main'
                    ELSE 'linuxdo'
                END AS provider_key,
                CASE
                    WHEN LOWER(TRIM(COALESCE(uei.provider, ''))) = 'wechat' THEN TRIM(COALESCE(uei.provider_union_id, ''))
                    ELSE TRIM(COALESCE(uei.provider_user_id, ''))
                END AS provider_subject,
                TRIM(COALESCE(uei.provider_user_id, '')) AS provider_user_id,
                TRIM(COALESCE(uei.provider_union_id, '')) AS provider_union_id,
                TRIM(COALESCE(uei.provider_username, '')) AS provider_username,
                TRIM(COALESCE(uei.display_name, '')) AS display_name,
                CASE
                    WHEN uei.metadata IS NULL OR TRIM(uei.metadata) = '' THEN CAST('{}' AS JSON)
                    WHEN NOT JSON_VALID(uei.metadata) THEN CAST('{}' AS JSON)
                    WHEN JSON_TYPE(uei.metadata) = 'OBJECT' THEN CAST(uei.metadata AS JSON)
                    ELSE JSON_OBJECT('_legacy_metadata_raw_json', CAST(uei.metadata AS JSON))
                END AS metadata_json,
                COALESCE(uei.updated_at, uei.created_at, NOW(3)) AS verified_at
            FROM user_external_identities AS uei
            JOIN users AS u ON u.id = uei.user_id
            WHERE u.deleted_at IS NULL
              AND LOWER(TRIM(COALESCE(uei.provider, ''))) IN ('linuxdo', 'wechat')
              AND (
                  (LOWER(TRIM(COALESCE(uei.provider, ''))) = 'linuxdo' AND TRIM(COALESCE(uei.provider_user_id, '')) <> '')
                  OR
                  (LOWER(TRIM(COALESCE(uei.provider, ''))) = 'wechat' AND TRIM(COALESCE(uei.provider_union_id, '')) <> '')
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
        SELECT
            legacy.user_id,
            legacy.provider_type,
            legacy.provider_key,
            legacy.provider_subject,
            legacy.verified_at,
            JSON_MERGE_PATCH(
                legacy.metadata_json,
                JSON_OBJECT(
                    'legacy_identity_id', legacy.id,
                    'provider_user_id', legacy.provider_user_id,
                    'provider_union_id', NULLIF(legacy.provider_union_id, ''),
                    'provider_username', legacy.provider_username,
                    'display_name', legacy.display_name,
                    'migration', '116_auth_identity_legacy_external_safety_reports'
                )
            )
        FROM canonical_legacy AS legacy
        LEFT JOIN auth_identities AS ai
          ON ai.provider_type = legacy.provider_type
         AND ai.provider_key = legacy.provider_key
         AND ai.provider_subject = legacy.provider_subject
        WHERE legacy.canonical_row_num = 1
          AND ai.id IS NULL
        ON DUPLICATE KEY UPDATE id = id;

        -- 5. 遗留渠道 subject 已归属其他用户 → 渠道冲突报告
        INSERT INTO auth_identity_migration_reports (report_type, report_key, details)
        SELECT
            'legacy_external_channel_conflict',
            CONCAT('legacy_external_identity:', legacy.id),
            JSON_MERGE_PATCH(
                legacy.metadata_json,
                JSON_OBJECT(
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
            )
        FROM (
            SELECT
                raw.id,
                raw.user_id,
                raw.provider_user_id,
                raw.provider_union_id,
                raw.metadata_json,
                TRIM(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(raw.metadata_json, '$.channel')), '')) AS channel,
                TRIM(COALESCE(
                    JSON_UNQUOTE(JSON_EXTRACT(raw.metadata_json, '$.channel_app_id')),
                    JSON_UNQUOTE(JSON_EXTRACT(raw.metadata_json, '$.appid')),
                    JSON_UNQUOTE(JSON_EXTRACT(raw.metadata_json, '$.app_id')),
                    ''
                )) AS channel_app_id
            FROM (
                SELECT
                    uei.id,
                    uei.user_id,
                    TRIM(COALESCE(uei.provider_user_id, '')) AS provider_user_id,
                    TRIM(COALESCE(uei.provider_union_id, '')) AS provider_union_id,
                    CASE
                        WHEN uei.metadata IS NULL OR TRIM(uei.metadata) = '' THEN CAST('{}' AS JSON)
                        WHEN NOT JSON_VALID(uei.metadata) THEN CAST('{}' AS JSON)
                        WHEN JSON_TYPE(uei.metadata) = 'OBJECT' THEN CAST(uei.metadata AS JSON)
                        ELSE JSON_OBJECT('_legacy_metadata_raw_json', CAST(uei.metadata AS JSON))
                    END AS metadata_json
                FROM user_external_identities AS uei
                JOIN users AS u ON u.id = uei.user_id
                WHERE u.deleted_at IS NULL
                  AND LOWER(TRIM(COALESCE(uei.provider, ''))) = 'wechat'
                  AND TRIM(COALESCE(uei.provider_union_id, '')) <> ''
                  AND TRIM(COALESCE(uei.provider_user_id, '')) <> ''
            ) AS raw
        ) AS legacy
        JOIN (
            SELECT
                TRIM(COALESCE(suei.provider_union_id, '')) AS provider_subject
            FROM user_external_identities AS suei
            JOIN users AS su ON su.id = suei.user_id
            WHERE su.deleted_at IS NULL
              AND LOWER(TRIM(COALESCE(suei.provider, ''))) = 'wechat'
              AND TRIM(COALESCE(suei.provider_union_id, '')) <> ''
              AND TRIM(COALESCE(suei.provider_user_id, '')) <> ''
            GROUP BY TRIM(COALESCE(suei.provider_union_id, ''))
            HAVING COUNT(DISTINCT suei.user_id) = 1
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
        ON DUPLICATE KEY UPDATE id = id;

        -- 6. 唯一归属 subject 的遗留渠道 → 回填 auth_identity_channels
        INSERT INTO auth_identity_channels (
            identity_id,
            provider_type,
            provider_key,
            channel,
            channel_app_id,
            channel_subject,
            metadata
        )
        WITH legacy_raw AS (
            SELECT
                uei.user_id,
                TRIM(COALESCE(uei.provider_user_id, '')) AS provider_user_id,
                TRIM(COALESCE(uei.provider_union_id, '')) AS provider_union_id,
                CASE
                    WHEN uei.metadata IS NULL OR TRIM(uei.metadata) = '' THEN CAST('{}' AS JSON)
                    WHEN NOT JSON_VALID(uei.metadata) THEN CAST('{}' AS JSON)
                    WHEN JSON_TYPE(uei.metadata) = 'OBJECT' THEN CAST(uei.metadata AS JSON)
                    ELSE JSON_OBJECT('_legacy_metadata_raw_json', CAST(uei.metadata AS JSON))
                END AS metadata_json
            FROM user_external_identities AS uei
            JOIN users AS u ON u.id = uei.user_id
            WHERE u.deleted_at IS NULL
              AND LOWER(TRIM(COALESCE(uei.provider, ''))) = 'wechat'
              AND TRIM(COALESCE(uei.provider_union_id, '')) <> ''
              AND TRIM(COALESCE(uei.provider_user_id, '')) <> ''
        ),
        legacy AS (
            SELECT
                raw.*,
                TRIM(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(raw.metadata_json, '$.channel')), '')) AS channel,
                TRIM(COALESCE(
                    JSON_UNQUOTE(JSON_EXTRACT(raw.metadata_json, '$.channel_app_id')),
                    JSON_UNQUOTE(JSON_EXTRACT(raw.metadata_json, '$.appid')),
                    JSON_UNQUOTE(JSON_EXTRACT(raw.metadata_json, '$.app_id')),
                    ''
                )) AS channel_app_id
            FROM legacy_raw AS raw
        ),
        clear_subjects AS (
            SELECT
                provider_union_id AS provider_subject
            FROM legacy
            GROUP BY provider_union_id
            HAVING COUNT(DISTINCT user_id) = 1
        )
        SELECT
            legacy_ai.id,
            'wechat',
            'wechat-main',
            legacy.channel,
            legacy.channel_app_id,
            legacy.provider_user_id,
            JSON_MERGE_PATCH(
                legacy.metadata_json,
                JSON_OBJECT(
                    'openid', legacy.provider_user_id,
                    'unionid', legacy.provider_union_id,
                    'migration', '116_auth_identity_legacy_external_safety_reports'
                )
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
        ON DUPLICATE KEY UPDATE id = id;

        -- 7. 只有 openid、无法离线归并的 wechat 遗留行 → 补救报告
        INSERT INTO auth_identity_migration_reports (report_type, report_key, details)
        SELECT
            'wechat_openid_only_requires_remediation',
            CONCAT('legacy_external_identity:', legacy.id),
            JSON_MERGE_PATCH(
                legacy.metadata_json,
                JSON_OBJECT(
                    'legacy_identity_id', legacy.id,
                    'user_id', legacy.user_id,
                    'openid', legacy.provider_user_id,
                    'reason', 'legacy user_external_identities row only has openid and cannot be canonicalized offline',
                    'migration', '116_auth_identity_legacy_external_safety_reports'
                )
            )
        FROM (
            SELECT
                uei.id,
                uei.user_id,
                TRIM(COALESCE(uei.provider_user_id, '')) AS provider_user_id,
                CASE
                    WHEN uei.metadata IS NULL OR TRIM(uei.metadata) = '' THEN CAST('{}' AS JSON)
                    WHEN NOT JSON_VALID(uei.metadata) THEN CAST('{}' AS JSON)
                    WHEN JSON_TYPE(uei.metadata) = 'OBJECT' THEN CAST(uei.metadata AS JSON)
                    ELSE JSON_OBJECT('_legacy_metadata_raw_json', CAST(uei.metadata AS JSON))
                END AS metadata_json
            FROM user_external_identities AS uei
            JOIN users AS u ON u.id = uei.user_id
            WHERE u.deleted_at IS NULL
              AND LOWER(TRIM(COALESCE(uei.provider, ''))) = 'wechat'
              AND TRIM(COALESCE(uei.provider_user_id, '')) <> ''
              AND TRIM(COALESCE(uei.provider_union_id, '')) = ''
        ) AS legacy
        ON DUPLICATE KEY UPDATE id = id;

    END IF;
END;

CALL __migration_116_tmp();
DROP PROCEDURE __migration_116_tmp;

-- metadata 必须是 JSON object（MySQL 8.0.16+ 强制执行；对应 PG 的 jsonb_typeof CHECK）
ALTER TABLE auth_identities
    ADD CONSTRAINT auth_identities_metadata_is_object_check
    CHECK (JSON_TYPE(metadata) = 'OBJECT');

ALTER TABLE auth_identity_channels
    ADD CONSTRAINT auth_identity_channels_metadata_is_object_check
    CHECK (JSON_TYPE(metadata) = 'OBJECT');

ALTER TABLE auth_identity_migration_reports
    ADD CONSTRAINT auth_identity_migration_reports_details_is_object_check
    CHECK (JSON_TYPE(details) = 'OBJECT');
