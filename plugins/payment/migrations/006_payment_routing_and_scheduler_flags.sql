INSERT INTO settings (key, value)
VALUES
    ('payment_visible_method_alipay_source', ''),
    ('payment_visible_method_wxpay_source', ''),
    ('payment_visible_method_alipay_enabled', 'false'),
    ('payment_visible_method_wxpay_enabled', 'false'),
    ('openai_advanced_scheduler_enabled', 'false')
ON CONFLICT (key) DO NOTHING;
