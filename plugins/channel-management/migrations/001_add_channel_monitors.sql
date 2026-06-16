-- Migration: 125_add_channel_monitors
-- 渠道监控 MVP：周期性对外部 provider/endpoint/api_key 做模型心跳测试。
--
-- 表结构说明：
--   - channel_monitors        渠道配置表（一行 = 一个监控对象）
--   - channel_monitor_histories 检测历史明细表（一次检测一个模型 = 一行）
--
-- 设计要点：
--   - api_key_encrypted 列存放 AES-256-GCM 密文（base64），由 service 层加密。
--   - extra_models 用 JSONB 存储字符串数组，便于扩展（后续可加权重等元数据）。
--   - history 表通过 ON DELETE CASCADE 自动清理已删除监控的历史。
--   - (enabled, last_checked_at) 索引服务于调度器扫描“到期需要检测”的监控。
--   - histories 上 (monitor_id, model, checked_at DESC) 服务用户视图聚合查询；
--     单独的 (checked_at) 索引服务定期清理 30 天前数据的 DELETE。

CREATE TABLE IF NOT EXISTS channel_monitors (
    id                BIGSERIAL PRIMARY KEY,
    name              VARCHAR(100) NOT NULL,
    provider          VARCHAR(20)  NOT NULL,    -- openai / anthropic / gemini
    endpoint          VARCHAR(500) NOT NULL,    -- base origin
    api_key_encrypted TEXT         NOT NULL,    -- AES-256-GCM (base64)
    primary_model     VARCHAR(200) NOT NULL,
    extra_models      JSONB        NOT NULL DEFAULT '[]'::jsonb,
    group_name        VARCHAR(100) NOT NULL DEFAULT '',
    enabled           BOOLEAN      NOT NULL DEFAULT TRUE,
    interval_seconds  INT          NOT NULL,
    last_checked_at   TIMESTAMPTZ,
    created_by        BIGINT       NOT NULL,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT channel_monitors_provider_check CHECK (provider IN ('openai', 'anthropic', 'gemini')),
    CONSTRAINT channel_monitors_interval_check CHECK (interval_seconds BETWEEN 15 AND 3600)
);

CREATE INDEX IF NOT EXISTS idx_channel_monitors_enabled_last_checked
    ON channel_monitors (enabled, last_checked_at);
CREATE INDEX IF NOT EXISTS idx_channel_monitors_provider
    ON channel_monitors (provider);
CREATE INDEX IF NOT EXISTS idx_channel_monitors_group_name
    ON channel_monitors (group_name);

CREATE TABLE IF NOT EXISTS channel_monitor_histories (
    id              BIGSERIAL PRIMARY KEY,
    monitor_id      BIGINT      NOT NULL REFERENCES channel_monitors(id) ON DELETE CASCADE,
    model           VARCHAR(200) NOT NULL,
    status          VARCHAR(20)  NOT NULL,
    latency_ms      INT,
    ping_latency_ms INT,
    message         VARCHAR(500) NOT NULL DEFAULT '',
    checked_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT channel_monitor_histories_status_check
        CHECK (status IN ('operational', 'degraded', 'failed', 'error'))
);

CREATE INDEX IF NOT EXISTS idx_channel_monitor_histories_monitor_model_checked
    ON channel_monitor_histories (monitor_id, model, checked_at DESC);
CREATE INDEX IF NOT EXISTS idx_channel_monitor_histories_checked_at
    ON channel_monitor_histories (checked_at);
