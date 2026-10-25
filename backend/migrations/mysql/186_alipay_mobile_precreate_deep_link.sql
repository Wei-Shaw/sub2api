-- MySQL 方言版本（翻译自 ../186_alipay_mobile_precreate_deep_link.sql）
-- Mobile Alipay keeps the legacy WAP flow unless this opt-in is enabled.
--
-- 说明：`key` 为 MySQL 保留字需反引号；ON CONFLICT (key) DO NOTHING → ON DUPLICATE KEY UPDATE id = id；
--       NOW() → NOW(3)。
INSERT INTO settings (`key`, value, updated_at)
VALUES ('ALIPAY_MOBILE_PRECREATE_DEEP_LINK', 'false', NOW(3))
ON DUPLICATE KEY UPDATE id = id;
