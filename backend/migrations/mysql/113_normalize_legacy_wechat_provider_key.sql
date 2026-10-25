-- MySQL 方言版本（翻译自 ../113_normalize_legacy_wechat_provider_key.sql）
--
-- 翻译决策：
--   - UPDATE ... WHERE NOT EXISTS (SELECT ... FROM 同表 ...) 在 MySQL 触发 1093
--     （UPDATE 目标表不能出现在子查询 FROM 中）→ 改写为 LEFT JOIN 物化派生表反连接，
--     派生表在更新前物化，与 PG 单语句快照语义一致。
--   - jsonb || jsonb_build_object(...) → JSON_MERGE_PATCH(COALESCE(metadata, CAST('{}' AS JSON)),
--     JSON_OBJECT(...)）：所有右侧键值均为非 NULL 常量，与 PG || 顶层合并语义等价。
--   - jsonb_build_object → JSON_OBJECT；比较表达式值（same_user）以布尔序列化为 true/false。
--   - CAST(id AS TEXT) → CAST(id AS CHAR)。
--   - ON CONFLICT (report_type, report_key) DO NOTHING → ON DUPLICATE KEY UPDATE id = id。
--   - NOW() → NOW(3)。

-- 1) 归一 auth_identities：wechat → wechat-main（仅当不存在同 subject 的规范身份时）
UPDATE auth_identities AS ai
LEFT JOIN (
    SELECT DISTINCT provider_subject
    FROM auth_identities
    WHERE provider_type = 'wechat'
      AND provider_key = 'wechat-main'
) AS canon
  ON canon.provider_subject = ai.provider_subject
SET
    ai.provider_key = 'wechat-main',
    ai.metadata = JSON_MERGE_PATCH(
        COALESCE(ai.metadata, CAST('{}' AS JSON)),
        JSON_OBJECT(
            'legacy_provider_key', 'wechat',
            'normalized_by_migration', '113_normalize_legacy_wechat_provider_key'
        )
    ),
    ai.updated_at = NOW(3)
WHERE ai.provider_type = 'wechat'
  AND ai.provider_key = 'wechat'
  AND canon.provider_subject IS NULL;

-- 2) 归一 auth_identity_channels：wechat → wechat-main（仅当不存在同 (channel, app_id, subject) 的规范渠道时）
UPDATE auth_identity_channels AS channel
LEFT JOIN (
    SELECT DISTINCT channel, channel_app_id, channel_subject
    FROM auth_identity_channels
    WHERE provider_type = 'wechat'
      AND provider_key = 'wechat-main'
) AS canon
  ON canon.channel = channel.channel
 AND canon.channel_app_id = channel.channel_app_id
 AND canon.channel_subject = channel.channel_subject
SET
    channel.provider_key = 'wechat-main',
    channel.metadata = JSON_MERGE_PATCH(
        COALESCE(channel.metadata, CAST('{}' AS JSON)),
        JSON_OBJECT(
            'legacy_provider_key', 'wechat',
            'normalized_by_migration', '113_normalize_legacy_wechat_provider_key'
        )
    ),
    channel.updated_at = NOW(3)
WHERE channel.provider_type = 'wechat'
  AND channel.provider_key = 'wechat'
  AND canon.channel IS NULL;

-- 3) 身份冲突报告
INSERT INTO auth_identity_migration_reports (report_type, report_key, details)
SELECT
    'wechat_provider_key_conflict',
    CAST(ai.id AS CHAR),
    JSON_OBJECT(
        'legacy_identity_id', ai.id,
        'legacy_user_id', ai.user_id,
        'provider_subject', ai.provider_subject,
        'canonical_identity_id', canon.id,
        'canonical_user_id', canon.user_id,
        'same_user', (canon.user_id = ai.user_id),
        'migration', '113_normalize_legacy_wechat_provider_key'
    )
FROM auth_identities AS ai
JOIN auth_identities AS canon
  ON canon.provider_type = 'wechat'
 AND canon.provider_key = 'wechat-main'
 AND canon.provider_subject = ai.provider_subject
WHERE ai.provider_type = 'wechat'
  AND ai.provider_key = 'wechat'
ON DUPLICATE KEY UPDATE id = auth_identity_migration_reports.id;

-- 4) 渠道冲突报告
INSERT INTO auth_identity_migration_reports (report_type, report_key, details)
SELECT
    'wechat_channel_provider_key_conflict',
    CAST(channel.id AS CHAR),
    JSON_OBJECT(
        'legacy_channel_id', channel.id,
        'legacy_identity_id', channel.identity_id,
        'canonical_channel_id', canon.id,
        'canonical_identity_id', canon.identity_id,
        'channel', channel.channel,
        'channel_app_id', channel.channel_app_id,
        'channel_subject', channel.channel_subject,
        'same_user', COALESCE((legacy_identity.user_id = canonical_identity.user_id), FALSE),
        'migration', '113_normalize_legacy_wechat_provider_key'
    )
FROM auth_identity_channels AS channel
JOIN auth_identity_channels AS canon
  ON canon.provider_type = 'wechat'
 AND canon.provider_key = 'wechat-main'
 AND canon.channel = channel.channel
 AND canon.channel_app_id = channel.channel_app_id
 AND canon.channel_subject = channel.channel_subject
LEFT JOIN auth_identities AS legacy_identity
  ON legacy_identity.id = channel.identity_id
LEFT JOIN auth_identities AS canonical_identity
  ON canonical_identity.id = canon.identity_id
WHERE channel.provider_type = 'wechat'
  AND channel.provider_key = 'wechat'
ON DUPLICATE KEY UPDATE id = auth_identity_migration_reports.id;
