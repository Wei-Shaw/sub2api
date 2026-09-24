-- API Key 按上游来源（平台）细分的限额。
--
-- 背景：composite（混合）分组下一个 API Key 会同时路由到多个上游平台，但 key 上的
-- quota / rate_limit_5h / rate_limit_1d / rate_limit_7d 是单一金额池，无法区分来源。
-- 本迁移新增两部分：
--   1. api_keys.platform_limits：限额配置（JSONB，platform -> 限额），随 key 行一起读出，
--      未配置时为空 map，热路径零额外开销。
--   2. api_key_platform_usages：按 (api_key_id, platform) 的用量与窗口，列语义与
--      api_keys 上的同名列完全一致；仅对显式配置了平台限额的组合建行。

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS platform_limits JSONB;

COMMENT ON COLUMN api_keys.platform_limits IS '按上游平台细分的 key 子限额（USD）：{"openai":{"quota":10,"rate_limit_1d":5}}；NULL/空 = 仅受 key 级限额约束';

CREATE TABLE IF NOT EXISTS api_key_platform_usages (
    id               BIGSERIAL PRIMARY KEY,
    api_key_id       BIGINT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    platform         VARCHAR(32) NOT NULL CHECK (platform IN (
        'anthropic', 'openai', 'gemini', 'antigravity', 'grok',
        'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go'
    )),

    -- 该来源累计消费，对应 platform_limits[platform].quota
    quota_used       DECIMAL(20,8) NOT NULL DEFAULT 0,

    -- 滚动窗口用量，语义同 api_keys.usage_5h / usage_1d / usage_7d
    usage_5h         DECIMAL(20,8) NOT NULL DEFAULT 0,
    usage_1d         DECIMAL(20,8) NOT NULL DEFAULT 0,
    usage_7d         DECIMAL(20,8) NOT NULL DEFAULT 0,

    -- 窗口起点（NULL = 尚未初始化，判定为已过期）
    window_5h_start  TIMESTAMPTZ,
    window_1d_start  TIMESTAMPTZ,
    window_7d_start  TIMESTAMPTZ,

    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS apikeyplatformusage_api_key_id_platform
    ON api_key_platform_usages (api_key_id, platform);

CREATE INDEX IF NOT EXISTS apikeyplatformusage_api_key_id
    ON api_key_platform_usages (api_key_id);
