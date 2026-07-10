-- Day0 white-label copy cleanup for video gateway demo rows.
-- Keep prior migrations immutable; this migration only updates display text.

UPDATE video_provider_accounts
SET display_name = '演示通道'
WHERE provider = 'mock'
  AND display_name = 'Mock Provider';

UPDATE video_provider_accounts
SET display_name = replace(display_name, '未配置 Key', '未配置凭证')
WHERE display_name LIKE '%未配置 Key%';

UPDATE video_provider_accounts
SET metadata_json = replace(metadata_json::text, 'Key 未配置', '凭证未配置')::jsonb
WHERE metadata_json::text LIKE '%Key 未配置%';

UPDATE video_task_events
SET
  message = replace(replace(message, 'Key 未配置', '凭证未配置'), '未配置 Key', '未配置凭证'),
  payload_json = replace(replace(payload_json::text, 'Key 未配置', '凭证未配置'), '未配置 Key', '未配置凭证')::jsonb
WHERE message LIKE '%Key%'
   OR payload_json::text LIKE '%Key%';
