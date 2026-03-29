-- Create channels table for managing pricing channels.
-- A channel groups multiple groups together and provides custom model pricing.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

-- 渠道表
CREATE TABLE IF NOT EXISTS channels (
    id          BIGSERIAL    PRIMARY KEY,
    name        VARCHAR(100) NOT NULL,
    description TEXT         DEFAULT '',
    status      VARCHAR(20)  NOT NULL DEFAULT 'active',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- 渠道名称唯一索引
CREATE UNIQUE INDEX IF NOT EXISTS idx_channels_name ON channels (name);
CREATE INDEX IF NOT EXISTS idx_channels_status ON channels (status);

-- 渠道-分组关联表（每个分组只能属于一个渠道）
CREATE TABLE IF NOT EXISTS channel_groups (
    id          BIGSERIAL    PRIMARY KEY,
    channel_id  BIGINT       NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    group_id    BIGINT       NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_groups_group_id ON channel_groups (group_id);
CREATE INDEX IF NOT EXISTS idx_channel_groups_channel_id ON channel_groups (channel_id);

-- 渠道模型定价表（一条定价可绑定多个模型）
CREATE TABLE IF NOT EXISTS channel_model_pricing (
    id                 BIGSERIAL      PRIMARY KEY,
    channel_id         BIGINT         NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    models             JSONB          NOT NULL DEFAULT '[]',
    input_price        NUMERIC(20,12),
    output_price       NUMERIC(20,12),
    cache_write_price  NUMERIC(20,12),
    cache_read_price   NUMERIC(20,12),
    image_output_price NUMERIC(20,8),
    created_at         TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_channel_model_pricing_channel_id ON channel_model_pricing (channel_id);

COMMENT ON TABLE channels IS '渠道管理：关联多个分组，提供自定义模型定价';
COMMENT ON TABLE channel_groups IS '渠道-分组关联表：每个分组最多属于一个渠道';
COMMENT ON TABLE channel_model_pricing IS '渠道模型定价：一条定价可绑定多个模型，价格一致';
COMMENT ON COLUMN channel_model_pricing.models IS '绑定的模型列表，JSON 数组，如 ["claude-opus-4-6","claude-opus-4-6-thinking"]';
COMMENT ON COLUMN channel_model_pricing.input_price IS '每 token 输入价格（USD），NULL 表示使用默认';
COMMENT ON COLUMN channel_model_pricing.output_price IS '每 token 输出价格（USD），NULL 表示使用默认';
COMMENT ON COLUMN channel_model_pricing.cache_write_price IS '缓存写入每 token 价格，NULL 表示使用默认';
COMMENT ON COLUMN channel_model_pricing.cache_read_price IS '缓存读取每 token 价格，NULL 表示使用默认';
COMMENT ON COLUMN channel_model_pricing.image_output_price IS '图片输出价格（Gemini Image 等），NULL 表示使用默认';
