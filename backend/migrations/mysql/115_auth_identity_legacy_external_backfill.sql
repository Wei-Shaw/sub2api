-- MySQL 方言版本（翻译自 ../115_auth_identity_legacy_external_backfill.sql）
--
-- 翻译决策：
--   - PG 的防御性 JSON 解析函数 __migration_115_safe_legacy_metadata_jsonb 内联为 CASE 表达式，
--     语义逐分支对齐：NULL/空白或非法 JSON → {}；合法 JSON 且为 object → 原值；
--     其余（合法但非 object，如数组/标量）→ {"_legacy_metadata_raw_json": <原 JSON 值>}
--     （注意 PG 语义是「解析失败返回 {}，合法但非 object 才包裹」）。
--   - DO 块的 user_external_identities 存在性守卫（遗留表仅老库才有，全新安装不存在）
--     必须保留 → 用临时存储过程实现，CALL 后即 DROP（执行器整文件单次执行，不写 DELIMITER）。
--   - jsonb || → JSON_MERGE_PATCH（右侧同名键覆盖；若右侧键值为 SQL NULL 则该键被移除
--     而非置 JSON null，本文件所有键值均经 COALESCE/TRIM 处理、不会为 NULL，无此差异）。
--   - ON CONFLICT (provider_type, provider_key, provider_subject) DO NOTHING /
--     ON CONFLICT DO NOTHING → ON DUPLICATE KEY UPDATE id = id。
--   - BTRIM → TRIM；to_regclass 守卫 → information_schema.tables；
--     CROSS JOIN LATERAL（按行物化 metadata_json）→ 嵌套派生表。
--   - 文件末尾的 synthetic_auth_identity 报告语句不依赖遗留表，保持在过程外执行。

DROP PROCEDURE IF EXISTS __migration_115_tmp;
CREATE PROCEDURE __migration_115_tmp()
BEGIN
    -- 遗留表 user_external_identities 仅存在于历史库；全新安装直接跳过整个回填。
    IF EXISTS (
        SELECT 1
        FROM information_schema.tables
        WHERE table_schema = DATABASE()
          AND table_name = 'user_external_identities'
    ) THEN

        -- 1. linuxdo → auth_identities（同一 provider_subject 仅唯一归属用户时回填，
        --    多用户归属取 updated_at/created_at 最新一行；冲突行静默跳过）
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
                TRIM(uei.provider_user_id) AS provider_user_id,
                TRIM(uei.provider_username) AS provider_username,
                TRIM(uei.display_name) AS display_name,
                CASE
                    WHEN uei.metadata IS NULL OR TRIM(uei.metadata) = '' THEN CAST('{}' AS JSON)
                    WHEN NOT JSON_VALID(uei.metadata) THEN CAST('{}' AS JSON)
                    WHEN JSON_TYPE(uei.metadata) = 'OBJECT' THEN CAST(uei.metadata AS JSON)
                    ELSE JSON_OBJECT('_legacy_metadata_raw_json', CAST(uei.metadata AS JSON))
                END AS metadata_json,
                uei.created_at,
                uei.updated_at
            FROM user_external_identities AS uei
            JOIN users AS u ON u.id = uei.user_id
            WHERE u.deleted_at IS NULL
              AND LOWER(TRIM(COALESCE(uei.provider, ''))) = 'linuxdo'
              AND TRIM(COALESCE(uei.provider_user_id, '')) <> ''
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
                    ORDER BY COALESCE(legacy.updated_at, legacy.created_at, NOW(3)) DESC, legacy.id DESC
                ) AS canonical_row_num
            FROM legacy
            JOIN legacy_subjects AS subjects
              ON subjects.provider_subject = legacy.provider_user_id
             AND subjects.distinct_user_count = 1
        )
        SELECT
            legacy.user_id,
            'linuxdo',
            'linuxdo',
            legacy.provider_user_id,
            COALESCE(legacy.updated_at, legacy.created_at, NOW(3)),
            JSON_MERGE_PATCH(
                legacy.metadata_json,
                JSON_OBJECT(
                    'legacy_identity_id', legacy.id,
                    'provider_user_id', legacy.provider_user_id,
                    'provider_username', legacy.provider_username,
                    'display_name', legacy.display_name,
                    'migration', '115_auth_identity_legacy_external_backfill'
                )
            )
        FROM canonical_legacy AS legacy
        WHERE legacy.canonical_row_num = 1
        ON DUPLICATE KEY UPDATE id = auth_identities.id;

        -- 2. wechat（unionid）→ auth_identities
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
                TRIM(uei.provider_user_id) AS provider_user_id,
                TRIM(uei.provider_union_id) AS provider_union_id,
                TRIM(uei.provider_username) AS provider_username,
                TRIM(uei.display_name) AS display_name,
                CASE
                    WHEN uei.metadata IS NULL OR TRIM(uei.metadata) = '' THEN CAST('{}' AS JSON)
                    WHEN NOT JSON_VALID(uei.metadata) THEN CAST('{}' AS JSON)
                    WHEN JSON_TYPE(uei.metadata) = 'OBJECT' THEN CAST(uei.metadata AS JSON)
                    ELSE JSON_OBJECT('_legacy_metadata_raw_json', CAST(uei.metadata AS JSON))
                END AS metadata_json,
                uei.created_at,
                uei.updated_at
            FROM user_external_identities AS uei
            JOIN users AS u ON u.id = uei.user_id
            WHERE u.deleted_at IS NULL
              AND LOWER(TRIM(COALESCE(uei.provider, ''))) = 'wechat'
              AND TRIM(COALESCE(uei.provider_union_id, '')) <> ''
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
                    ORDER BY COALESCE(legacy.updated_at, legacy.created_at, NOW(3)) DESC, legacy.id DESC
                ) AS canonical_row_num
            FROM legacy
            JOIN legacy_subjects AS subjects
              ON subjects.provider_subject = legacy.provider_union_id
             AND subjects.distinct_user_count = 1
        )
        SELECT
            legacy.user_id,
            'wechat',
            'wechat-main',
            legacy.provider_union_id,
            COALESCE(legacy.updated_at, legacy.created_at, NOW(3)),
            JSON_MERGE_PATCH(
                legacy.metadata_json,
                JSON_OBJECT(
                    'legacy_identity_id', legacy.id,
                    'openid', legacy.provider_user_id,
                    'unionid', legacy.provider_union_id,
                    'provider_user_id', legacy.provider_user_id,
                    'provider_union_id', legacy.provider_union_id,
                    'provider_username', legacy.provider_username,
                    'display_name', legacy.display_name,
                    'migration', '115_auth_identity_legacy_external_backfill'
                )
            )
        FROM canonical_legacy AS legacy
        WHERE legacy.canonical_row_num = 1
        ON DUPLICATE KEY UPDATE id = auth_identities.id;

        -- 3. wechat 渠道信息 → auth_identity_channels（仅唯一归属用户）
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
                TRIM(uei.provider_user_id) AS provider_user_id,
                TRIM(uei.provider_union_id) AS provider_union_id,
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
        legacy_subjects AS (
            SELECT
                provider_union_id AS provider_subject,
                COUNT(DISTINCT user_id) AS distinct_user_count
            FROM legacy
            GROUP BY provider_union_id
        )
        SELECT
            ai.id,
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
                    'migration', '115_auth_identity_legacy_external_backfill'
                )
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
        ON DUPLICATE KEY UPDATE id = auth_identity_channels.id;

        -- 4. 只有 openid、无法离线归并的 wechat 遗留行 → 补救报告
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
                    'migration', '115_auth_identity_legacy_external_backfill'
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
        ON DUPLICATE KEY UPDATE id = auth_identity_migration_reports.id;

    END IF;
END;

CALL __migration_115_tmp();
DROP PROCEDURE __migration_115_tmp;

-- 合成邮箱 wechat 身份仍缺 unionid 元数据 → 补救报告（不依赖遗留表，过程外执行）
INSERT INTO auth_identity_migration_reports (report_type, report_key, details)
SELECT
    'wechat_openid_only_requires_remediation',
    CONCAT('synthetic_auth_identity:', ai.id),
    JSON_MERGE_PATCH(
        COALESCE(ai.metadata, CAST('{}' AS JSON)),
        JSON_OBJECT(
            'auth_identity_id', ai.id,
            'user_id', ai.user_id,
            'provider_subject', ai.provider_subject,
            'reason', 'synthetic wechat auth identity still lacks unionid metadata and needs remediation',
            'migration', '115_auth_identity_legacy_external_backfill'
        )
    )
FROM auth_identities AS ai
WHERE ai.provider_type = 'wechat'
  AND COALESCE(JSON_UNQUOTE(JSON_EXTRACT(ai.metadata, '$.backfill_source')), '') = 'synthetic_email'
  AND TRIM(COALESCE(JSON_UNQUOTE(JSON_EXTRACT(ai.metadata, '$.unionid')), '')) = ''
ON DUPLICATE KEY UPDATE id = auth_identity_migration_reports.id;
