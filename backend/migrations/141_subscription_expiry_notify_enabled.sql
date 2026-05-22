-- 订阅到期提醒邮件开关，默认保持历史行为：开启。
INSERT INTO settings (key, value, updated_at)
VALUES ('subscription_expiry_notify_enabled', 'true', NOW())
ON CONFLICT (key) DO NOTHING;
