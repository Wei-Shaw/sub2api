-- MySQL 方言版本（翻译自 ../141_subscription_expiry_notify_enabled.sql）
-- 翻译说明：`key` 为 MySQL 保留字加反引号；NOW() → NOW(3)；
-- ON CONFLICT (key) DO NOTHING → ON DUPLICATE KEY UPDATE id = id（settings 有 id 列）。

-- 订阅到期提醒邮件开关，默认保持历史行为：开启。
INSERT INTO settings (`key`, value, updated_at)
VALUES ('subscription_expiry_notify_enabled', 'true', NOW(3))
ON DUPLICATE KEY UPDATE id = id;
