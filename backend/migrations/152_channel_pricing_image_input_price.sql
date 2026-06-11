ALTER TABLE channel_model_pricing
    ADD COLUMN IF NOT EXISTS image_input_price NUMERIC(20,10);

ALTER TABLE channel_account_stats_model_pricing
    ADD COLUMN IF NOT EXISTS image_input_price NUMERIC(20,10);

COMMENT ON COLUMN channel_model_pricing.image_input_price IS '图片输入 token 价格（USD），NULL 表示使用默认';
COMMENT ON COLUMN channel_account_stats_model_pricing.image_input_price IS '账号统计自定义规则的图片输入 token 价格（USD），NULL 表示使用输入价格';
