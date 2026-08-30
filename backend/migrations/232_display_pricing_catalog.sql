-- 用户可见的展示定价目录。
--
-- 这些表只服务于“模型价格”页面，不被网关、渠道解析器或 BillingService 引用。
-- 因而管理员修改这里的倍率/价格不会改变真实扣费或上游成本。

CREATE TABLE IF NOT EXISTS display_pricing_settings (
    id SMALLINT PRIMARY KEY DEFAULT 1,
    global_multiplier NUMERIC(12, 6) NOT NULL DEFAULT 1.000000,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT display_pricing_settings_singleton CHECK (id = 1),
    CONSTRAINT display_pricing_settings_multiplier_positive CHECK (global_multiplier > 0)
);

INSERT INTO display_pricing_settings (id, global_multiplier)
VALUES (1, 1.000000)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS display_pricing_providers (
    provider VARCHAR(64) PRIMARY KEY,
    display_name VARCHAR(100) NOT NULL,
    currency VARCHAR(3) NOT NULL,
    multiplier NUMERIC(12, 6),
    logo_key VARCHAR(64) NOT NULL DEFAULT '',
    logo_url TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT display_pricing_providers_currency_check CHECK (currency IN ('CNY', 'USD')),
    CONSTRAINT display_pricing_providers_multiplier_positive CHECK (multiplier IS NULL OR multiplier > 0)
);

INSERT INTO display_pricing_providers (provider, display_name, currency, multiplier, logo_key, logo_url, sort_order) VALUES
    ('auto', 'Auto', 'CNY', 0.125000, 'auto', '', 10),
    ('deepseek', 'DeepSeek', 'CNY', 0.125000, 'deepseek', '', 20),
    ('zhipu', 'GLM', 'CNY', 0.125000, 'zhipu', '', 30),
    ('moonshot', 'Kimi', 'CNY', 0.125000, 'kimi', '', 40),
    ('minimax', 'MiniMax', 'CNY', 0.125000, 'minimax', '', 50),
    ('qwen', 'Qwen', 'CNY', 0.125000, 'qwen', '', 60),
    ('mimo', 'MiMo', 'CNY', 0.125000, 'mimo', '', 70),
    ('hunyuan', 'Hunyuan', 'CNY', 0.125000, 'hunyuan', '', 80),
    ('openai', 'OpenAI', 'USD', 0.043750, 'openai', '', 100),
    ('anthropic', 'Anthropic', 'USD', 0.043750, 'anthropic', '', 110),
    ('gemini', 'Gemini', 'USD', 0.043750, 'gemini', '', 120),
    ('grok', 'Grok', 'USD', 0.043750, 'grok', '', 130)
ON CONFLICT (provider) DO NOTHING;

CREATE TABLE IF NOT EXISTS display_model_prices (
    id BIGSERIAL PRIMARY KEY,
    platform VARCHAR(64) NOT NULL,
    model_name VARCHAR(255) NOT NULL,
    provider VARCHAR(64) NOT NULL REFERENCES display_pricing_providers(provider) ON UPDATE CASCADE ON DELETE CASCADE,
    billing_mode VARCHAR(24) NOT NULL,
    currency VARCHAR(3) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INTEGER NOT NULL DEFAULT 0,

    -- Token 官方价格快照，单位为“每百万 token”。
    -- 模型倍率非空时是绝对固定倍率；否则最终展示倍率 = 全局倍率 * (厂商倍率 ?? 1)。
    official_input_per_million NUMERIC(20, 8),
    official_output_per_million NUMERIC(20, 8),
    official_cache_write_per_million NUMERIC(20, 8),
    official_cache_read_per_million NUMERIC(20, 8),
    model_multiplier NUMERIC(12, 6),

    -- 按次仅保存首档与可选覆盖值，不参与任何倍率计算。
    per_request_lte_256k NUMERIC(20, 8),
    per_request_256k_512k_override NUMERIC(20, 8),
    per_request_gt_512k_override NUMERIC(20, 8),

    -- 生图展示档位，例如 [{"label":"1K","price":0.04}]。
    image_prices JSONB NOT NULL DEFAULT '[]'::jsonb,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT display_model_prices_identity_unique UNIQUE (platform, model_name, billing_mode),
    CONSTRAINT display_model_prices_billing_mode_check CHECK (billing_mode IN ('token', 'per_request', 'image')),
    CONSTRAINT display_model_prices_currency_check CHECK (currency IN ('CNY', 'USD')),
    CONSTRAINT display_model_prices_model_multiplier_positive CHECK (model_multiplier IS NULL OR model_multiplier > 0),
    CONSTRAINT display_model_prices_nonnegative CHECK (
        (official_input_per_million IS NULL OR official_input_per_million >= 0) AND
        (official_output_per_million IS NULL OR official_output_per_million >= 0) AND
        (official_cache_write_per_million IS NULL OR official_cache_write_per_million >= 0) AND
        (official_cache_read_per_million IS NULL OR official_cache_read_per_million >= 0) AND
        (per_request_lte_256k IS NULL OR per_request_lte_256k >= 0) AND
        (per_request_256k_512k_override IS NULL OR per_request_256k_512k_override >= 0) AND
        (per_request_gt_512k_override IS NULL OR per_request_gt_512k_override >= 0)
    ),
    CONSTRAINT display_model_prices_mode_columns_check CHECK (
        (billing_mode = 'token' AND per_request_lte_256k IS NULL AND image_prices = '[]'::jsonb) OR
        (billing_mode = 'per_request' AND model_multiplier IS NULL AND
            official_input_per_million IS NULL AND official_output_per_million IS NULL AND
            official_cache_write_per_million IS NULL AND official_cache_read_per_million IS NULL AND
            per_request_lte_256k IS NOT NULL AND image_prices = '[]'::jsonb) OR
        (billing_mode = 'image' AND
            official_input_per_million IS NULL AND official_output_per_million IS NULL AND
            official_cache_write_per_million IS NULL AND official_cache_read_per_million IS NULL AND
            per_request_lte_256k IS NULL AND jsonb_typeof(image_prices) = 'array')
    )
);

CREATE INDEX IF NOT EXISTS idx_display_model_prices_enabled_provider
    ON display_model_prices (enabled, provider, sort_order, model_name);

-- 已确认的按次目录。首档是独立展示售价；其余两档由后端默认派生为 1.5x / 2x。
-- 这里不保存、不计算任何倍率。
INSERT INTO display_model_prices
    (platform, model_name, provider, billing_mode, currency, per_request_lte_256k, sort_order)
VALUES
    ('openai', 'Auto-Model', 'auto', 'per_request', 'CNY', 0.00135, 10),
    ('openai', 'deepseek-v4-flash-0731', 'deepseek', 'per_request', 'CNY', 0.00675, 20),
    ('openai', 'deepseek-v4-pro-0813', 'deepseek', 'per_request', 'CNY', 0.0081, 21),
    ('openai', 'glm-5.1', 'zhipu', 'per_request', 'CNY', 0.0081, 30),
    ('openai', 'glm-5.2', 'zhipu', 'per_request', 'CNY', 0.0081, 31),
    ('openai', 'glm-5.3', 'zhipu', 'per_request', 'CNY', 0.0135, 32),
    ('openai', 'glm-5.3-flash', 'zhipu', 'per_request', 'CNY', 0.0081, 33),
    ('grok', 'grok-4.6', 'grok', 'per_request', 'USD', 0.001875, 40),
    ('openai', 'kimi-k2.6', 'moonshot', 'per_request', 'CNY', 0.0081, 50),
    ('openai', 'kimi-k2.7-code', 'moonshot', 'per_request', 'CNY', 0.0135, 51),
    ('openai', 'MiniMax-M2.7', 'minimax', 'per_request', 'CNY', 0.0027, 60),
    ('openai', 'MiniMax-M2.7-highspeed', 'minimax', 'per_request', 'CNY', 0.00405, 61),
    ('openai', 'MiniMax-M3', 'minimax', 'per_request', 'CNY', 0.00675, 62),
    ('openai', 'gpt-5.6', 'openai', 'per_request', 'USD', 0.001875, 70)
ON CONFLICT (platform, model_name, billing_mode) DO NOTHING;

-- 33 个已确认目录模型的官方 token 基础价快照（每百万 token）。
-- 未经来源确认的价格保持 NULL，管理员补齐后才会出现数值，禁止伪造官方价。
INSERT INTO display_model_prices
    (platform, model_name, provider, billing_mode, currency,
     official_input_per_million, official_output_per_million,
     official_cache_write_per_million, official_cache_read_per_million, model_multiplier, sort_order)
VALUES
    ('openai', 'deepseek-v4-flash-0731', 'deepseek', 'token', 'CNY', 1.6, 4.7, NULL, 0.1, NULL, 100),
    ('openai', 'deepseek-v4-pro-0813', 'deepseek', 'token', 'CNY', 4.7, 13.9, NULL, 0.2, NULL, 101),
    ('openai', 'deepseek-v4-flash-vision-exp', 'deepseek', 'token', 'CNY', 1.6, 4.7, NULL, 0.1, 0.46875, 102),
    ('openai', 'kimi-k2.6', 'moonshot', 'token', 'CNY', 6.7, 28, NULL, 1.2, NULL, 110),
    ('openai', 'kimi-k2.7-code', 'moonshot', 'token', 'CNY', 6.7, 28, NULL, 1.4, NULL, 111),
    ('openai', 'kimi-k3', 'moonshot', 'token', 'CNY', 21, 105, NULL, 2.1, NULL, 112),
    ('openai', 'glm-5.1', 'zhipu', 'token', 'CNY', 9.8, 30.9, NULL, 1.9, NULL, 120),
    ('openai', 'glm-5.2', 'zhipu', 'token', 'CNY', 9.8, 30.9, NULL, 1.9, NULL, 121),
    ('openai', 'glm-5.3', 'zhipu', 'token', 'CNY', 9.8, 30.9, NULL, 1.9, NULL, 122),
    ('openai', 'glm-5.3-flash', 'zhipu', 'token', 'CNY', 1.1, 3.5, NULL, 0.3, NULL, 123),
    ('openai', 'MiniMax-M2.7', 'minimax', 'token', 'CNY', 2.1, 8.4, 2.7, 0.5, NULL, 130),
    ('openai', 'MiniMax-M2.7-highspeed', 'minimax', 'token', 'CNY', 2.1, 8.4, NULL, 0.5, NULL, 131),
    ('openai', 'MiniMax-M3', 'minimax', 'token', 'CNY', 2.1, 8.4, NULL, 0.5, NULL, 132),
    ('openai', 'qwen3.7-max', 'qwen', 'token', 'CNY', 17.5, 52.5, 21.9, 3.5, NULL, 140),
    ('openai', 'qwen3.8-max', 'qwen', 'token', 'CNY', 14, 42, 17.5, 1.8, NULL, 141),
    ('openai', 'mimo-v2.5', 'mimo', 'token', 'CNY', 1, 2, NULL, 0.1, NULL, 150),
    ('openai', 'mimo-v2.5-pro', 'mimo', 'token', 'CNY', 3.1, 6.1, NULL, 0.1, NULL, 151),
    ('openai', 'hy3', 'hunyuan', 'token', 'CNY', 1, 4.1, NULL, 0.3, NULL, 160),
    ('anthropic', 'claude-opus-4-7', 'anthropic', 'token', 'USD', 35, 175, 43.8, 3.5, NULL, 200),
    ('anthropic', 'claude-opus-4-8', 'anthropic', 'token', 'USD', 35, 175, 43.8, 3.5, NULL, 201),
    ('anthropic', 'claude-sonnet-5', 'anthropic', 'token', 'USD', 21, 105, 24.5, 2.1, NULL, 202),
    ('anthropic', 'claude-fable-5', 'anthropic', 'token', 'USD', 70, 350, 87.5, 7, NULL, 203),
    ('anthropic', 'claude-opus-5', 'anthropic', 'token', 'USD', 35, 175, 43.8, 3.5, NULL, 204),
    ('openai', 'gpt-5.5', 'openai', 'token', 'USD', 35, 210, NULL, 3.5, NULL, 210),
    ('openai', 'gpt-5.6-luna', 'openai', 'token', 'USD', 1.5, 8.4, 1.8, 0.2, NULL, 211),
    ('openai', 'gpt-5.6-sol', 'openai', 'token', 'USD', 35, 210, 43.8, 3.5, NULL, 212),
    ('openai', 'gpt-5.6-terra', 'openai', 'token', 'USD', 14, 84, 17.5, 1.4, NULL, 213),
    ('gemini', 'gemini-3.1-pro-preview', 'gemini', 'token', 'USD', 14, 84, NULL, 1.4, NULL, 220),
    ('gemini', 'gemini-3.5-flash', 'gemini', 'token', 'USD', 10.5, 63, NULL, 1.1, NULL, 221),
    ('gemini', 'gemini-3.6-flash', 'gemini', 'token', 'USD', 10.5, 52.5, 0.1, 1.1, NULL, 222),
    ('gemini', 'gemini-3.7-flash', 'gemini', 'token', 'USD', 5.3, 26.3, 0.1, 0.6, NULL, 223),
    ('grok', 'grok-4.5', 'grok', 'token', 'USD', 14, 42, NULL, 3.5, NULL, 230),
    ('grok', 'grok-4.6', 'grok', 'token', 'USD', 14, 42, NULL, 3.5, NULL, 231)
ON CONFLICT (platform, model_name, billing_mode) DO NOTHING;

-- gpt-image-2 为国外模型，展示币种固定 USD；价格档位可由管理员随时覆盖。
INSERT INTO display_model_prices
    (platform, model_name, provider, billing_mode, currency, image_prices, sort_order)
VALUES
    ('openai', 'gpt-image-2', 'openai', 'image', 'USD',
     '[{"label":"standard","price":0.04},{"label":"high","price":0.08}]'::jsonb, 300)
ON CONFLICT (platform, model_name, billing_mode) DO NOTHING;
