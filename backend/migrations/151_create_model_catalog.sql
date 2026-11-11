-- 模型目录：管理员可在后台登记模型（含新发布模型），无需发版。
--   * 已启用条目并入 /v1/models 回退列表与前端白名单/映射候选
--   * 配置了价格的条目作为全局默认价（渠道定价 > 模型目录 > LiteLLM > 内置回退）
-- 价格单位：USD / token（NULL 表示不覆盖）。

CREATE TABLE IF NOT EXISTS model_catalog (
    id                BIGSERIAL      PRIMARY KEY,
    model_id          VARCHAR(200)   NOT NULL,
    platform          VARCHAR(50)    NOT NULL,
    vendor            VARCHAR(50)    NOT NULL DEFAULT '',
    display_name      VARCHAR(200)   NOT NULL DEFAULT '',
    description       TEXT           NOT NULL DEFAULT '',
    input_price       NUMERIC(20,12),
    output_price      NUMERIC(20,12),
    cache_write_price NUMERIC(20,12),
    cache_read_price  NUMERIC(20,12),
    context_window    INTEGER        NOT NULL DEFAULT 0,
    max_output_tokens INTEGER        NOT NULL DEFAULT 0,
    released_at       DATE,
    enabled           BOOLEAN        NOT NULL DEFAULT TRUE,
    sort_order        INTEGER        NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_model_catalog_platform_model UNIQUE (platform, model_id)
);

CREATE INDEX IF NOT EXISTS idx_model_catalog_enabled_sort ON model_catalog (enabled, sort_order, id);

COMMENT ON TABLE model_catalog IS '模型目录：后台可配置的模型清单与全局默认价格';

-- Anthropic Claude 5 代（价格来源：Anthropic 官方定价，2026-11；缓存写入为 5 分钟 TTL）
INSERT INTO model_catalog (model_id, platform, vendor, display_name, input_price, output_price, cache_write_price, cache_read_price, context_window, max_output_tokens, released_at, sort_order)
VALUES
    ('claude-fable-5-2', 'anthropic', 'anthropic', 'Claude Fable 5.2', 8e-06, 4e-05, 1e-05, 2e-07, 1000000, 128000, NULL, 1),
    ('claude-fable-5-1', 'anthropic', 'anthropic', 'Claude Fable 5.1', 1e-05, 5e-05, 1.25e-05, 1e-06, 1000000, 128000, NULL, 2),
    ('claude-fable-5', 'anthropic', 'anthropic', 'Claude Fable 5', 1e-05, 5e-05, 1.25e-05, 1e-06, 1000000, 128000, '2026-06-09', 3),
    ('claude-opus-5-5', 'anthropic', 'anthropic', 'Claude Opus 5.5', 4e-06, 2e-05, 5e-06, 2e-07, 1000000, 128000, NULL, 4),
    ('claude-opus-5', 'anthropic', 'anthropic', 'Claude Opus 5', 5e-06, 2.5e-05, 6.25e-06, 5e-07, 1000000, 128000, NULL, 5),
    ('claude-sonnet-5-5', 'anthropic', 'anthropic', 'Claude Sonnet 5.5', 2e-06, 1e-05, 2.5e-06, 1e-07, 1000000, 128000, NULL, 6),
    ('claude-sonnet-5', 'anthropic', 'anthropic', 'Claude Sonnet 5', 2e-06, 1e-05, 2.5e-06, 2e-07, 1000000, 128000, NULL, 7),
    ('claude-haiku-5-5', 'anthropic', 'anthropic', 'Claude Haiku 5.5', 1e-07, 5e-07, 1.25e-07, 1e-08, 1000000, 128000, NULL, 8)
ON CONFLICT (platform, model_id) DO NOTHING;
