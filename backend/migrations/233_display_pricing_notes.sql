-- Presentation-only notes for the customer-facing model price catalogue.
-- These columns are deliberately isolated from channels, groups, and billing.

ALTER TABLE display_pricing_providers
    ADD COLUMN IF NOT EXISTS provider_note VARCHAR(4000) NOT NULL DEFAULT '';

ALTER TABLE display_model_prices
    ADD COLUMN IF NOT EXISTS model_note VARCHAR(1000) NOT NULL DEFAULT '';

UPDATE display_pricing_providers
SET provider_note = 'DeepSeek 平常价格展示；高峰期为工作日北京时间 09:00–12:00、14:00–18:00，高峰价格按平常价格 ×2 计算。'
WHERE provider = 'deepseek' AND provider_note = '';

UPDATE display_model_prices
SET model_note = '新模型上线初期资源较为紧张，当前价格偏高；待资源供应充足后将适时下调。'
WHERE model_name = 'deepseek-v4-flash-vision-exp' AND model_note = '';
