-- MySQL 方言版本（翻译自 ../138_channel_monitor_openai_api_mode.sql）
-- 翻译说明：
--   - ADD COLUMN IF NOT EXISTS → ADD COLUMN（按迁移执行器单次执行语义去掉）。
--   - DO 块（约束存在性检查 + ADD CONSTRAINT CHECK）：存在性分支去掉后直接执行，
--     两个 CHECK 约束均由本迁移首次引入，同名保留。

-- Migration: 137_channel_monitor_openai_api_mode
-- 为渠道监控和请求模板增加 OpenAI 协议模式：
--   chat_completions -> /v1/chat/completions + messages
--   responses        -> /v1/responses + instructions/input
-- 历史数据默认保持 chat_completions，避免改变现有监控行为。

ALTER TABLE channel_monitors
    ADD COLUMN api_mode VARCHAR(32) NOT NULL DEFAULT 'chat_completions';

ALTER TABLE channel_monitor_request_templates
    ADD COLUMN api_mode VARCHAR(32) NOT NULL DEFAULT 'chat_completions';

ALTER TABLE channel_monitors
    ADD CONSTRAINT channel_monitors_api_mode_check
    CHECK (api_mode IN ('chat_completions', 'responses'));

ALTER TABLE channel_monitor_request_templates
    ADD CONSTRAINT channel_monitor_request_templates_api_mode_check
    CHECK (api_mode IN ('chat_completions', 'responses'));

CREATE INDEX idx_channel_monitors_provider_api_mode
    ON channel_monitors (provider, api_mode);

CREATE INDEX idx_channel_monitor_templates_provider_api_mode
    ON channel_monitor_request_templates (provider, api_mode);
