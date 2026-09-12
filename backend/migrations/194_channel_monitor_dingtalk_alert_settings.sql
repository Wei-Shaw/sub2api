-- Seed DingTalk channel-monitor alert settings for existing installations.
-- The values are intentionally disabled/empty and never overwrite admin configuration.
INSERT INTO settings (key, value, updated_at)
VALUES
    ('channel_monitor_dingtalk_enabled', 'false', NOW()),
    ('channel_monitor_dingtalk_webhook', '', NOW()),
    ('channel_monitor_dingtalk_secret', '', NOW())
ON CONFLICT (key) DO NOTHING;
