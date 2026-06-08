UPDATE auth_identities AS ai
SET
    provider_key = 'wechat-main',
    metadata = COALESCE(ai.metadata, '{}'::jsonb) || jsonb_build_object(
        'legacy_provider_key', 'wechat',
        'normalized_by_migration', '113_normalize_legacy_wechat_provider_key'
    ),
    updated_at = NOW()
WHERE ai.provider_type = 'wechat'
  AND ai.provider_key = 'wechat'
  AND NOT EXISTS (
      SELECT 1
      FROM auth_identities AS canon
      WHERE canon.provider_type = 'wechat'
        AND canon.provider_key = 'wechat-main'
        AND canon.provider_subject = ai.provider_subject
  );

UPDATE auth_identity_channels AS channel
SET
    provider_key = 'wechat-main',
    metadata = COALESCE(channel.metadata, '{}'::jsonb) || jsonb_build_object(
        'legacy_provider_key', 'wechat',
        'normalized_by_migration', '113_normalize_legacy_wechat_provider_key'
    ),
    updated_at = NOW()
WHERE channel.provider_type = 'wechat'
  AND channel.provider_key = 'wechat'
  AND NOT EXISTS (
      SELECT 1
      FROM auth_identity_channels AS canon
      WHERE canon.provider_type = 'wechat'
        AND canon.provider_key = 'wechat-main'
        AND canon.channel = channel.channel
        AND canon.channel_app_id = channel.channel_app_id
        AND canon.channel_subject = channel.channel_subject
  );

INSERT INTO auth_identity_migration_reports (report_type, report_key, details)
SELECT
    'wechat_provider_key_conflict',
    CAST(ai.id AS TEXT),
    jsonb_build_object(
        'legacy_identity_id', ai.id,
        'legacy_user_id', ai.user_id,
        'provider_subject', ai.provider_subject,
        'canonical_identity_id', canon.id,
        'canonical_user_id', canon.user_id,
        'same_user', canon.user_id = ai.user_id,
        'migration', '113_normalize_legacy_wechat_provider_key'
    )
FROM auth_identities AS ai
JOIN auth_identities AS canon
  ON canon.provider_type = 'wechat'
 AND canon.provider_key = 'wechat-main'
 AND canon.provider_subject = ai.provider_subject
WHERE ai.provider_type = 'wechat'
  AND ai.provider_key = 'wechat'
ON CONFLICT (report_type, report_key) DO NOTHING;

INSERT INTO auth_identity_migration_reports (report_type, report_key, details)
SELECT
    'wechat_channel_provider_key_conflict',
    CAST(channel.id AS TEXT),
    jsonb_build_object(
        'legacy_channel_id', channel.id,
        'legacy_identity_id', channel.identity_id,
        'canonical_channel_id', canon.id,
        'canonical_identity_id', canon.identity_id,
        'channel', channel.channel,
        'channel_app_id', channel.channel_app_id,
        'channel_subject', channel.channel_subject,
        'same_user', COALESCE(legacy_identity.user_id = canonical_identity.user_id, FALSE),
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
ON CONFLICT (report_type, report_key) DO NOTHING;
