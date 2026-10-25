-- MySQL 方言版本（翻译自 ../178_channel_image_input_price.sql）
-- 翻译说明：ADD COLUMN IF NOT EXISTS → ADD COLUMN；NUMERIC(20,12) → DECIMAL(20,12)（同义类型）。

-- 178_channel_image_input_price.sql
-- 渠道自定义定价（token 模式）新增图片输入 token 单价列。
-- 用于 gpt-image-2 等模型的图片编辑/图生图请求：上游 usage 的
-- input_tokens_details.image_tokens 需按独立单价计费，区别于文本 input_price。
-- 未配置（NULL）时计费回退到文本输入价，保持向后兼容。

ALTER TABLE channel_model_pricing
    ADD COLUMN image_input_price DECIMAL(20,12) NULL;
