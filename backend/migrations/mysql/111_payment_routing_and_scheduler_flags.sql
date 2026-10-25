-- MySQL 方言版本（翻译自 ../111_payment_routing_and_scheduler_flags.sql）
-- ON CONFLICT (key) DO NOTHING → ON DUPLICATE KEY UPDATE id = id；`key` 为 MySQL 保留字，加反引号。

INSERT INTO settings (`key`, value)
VALUES
    ('payment_visible_method_alipay_source', ''),
    ('payment_visible_method_wxpay_source', ''),
    ('payment_visible_method_alipay_enabled', 'false'),
    ('payment_visible_method_wxpay_enabled', 'false'),
    ('openai_advanced_scheduler_enabled', 'false')
ON DUPLICATE KEY UPDATE id = id;
