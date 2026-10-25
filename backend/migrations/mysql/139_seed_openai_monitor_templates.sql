-- MySQL 方言版本（翻译自 ../139_seed_openai_monitor_templates.sql）
-- 翻译说明：
--   - ON CONFLICT (provider, name) DO NOTHING → ON DUPLICATE KEY UPDATE id = id
--     （表有 id 列；冲突目标为 128 迁移建的同名唯一索引
--     channel_monitor_request_templates_provider_name，且为该表除主键外唯一约束，
--     语义一致）。
--   - '{}'::jsonb / '{"max_tokens": 20}'::jsonb → JSON 列直接收字符串字面量（自动按 JSON 解析）。

-- Migration: 138_seed_openai_monitor_templates
-- 内置 OpenAI 渠道监控模板。重点是把协议模式显式化：
--   1) OpenAI-compatible 使用 Chat Completions payload
--   2) Responses / 本站自检 使用 Responses payload，默认 body 由后端 adapter 填入 instructions + input
-- 所有模板都可直接选择；ON CONFLICT 保证重复部署不覆盖用户编辑。

INSERT INTO channel_monitor_request_templates (
    name, provider, api_mode, description, extra_headers, body_override_mode, body_override
)
VALUES
(
    'OpenAI Compatible 默认检测',
    'openai',
    'chat_completions',
    '适用于大多数 OpenAI-compatible 上游：POST /v1/chat/completions，后端自动生成 messages 数学 challenge。',
    '{}',
    'off',
    NULL
),
(
    'OpenAI Compatible 低 token 检测',
    'openai',
    'chat_completions',
    '仍走 /v1/chat/completions，仅把 max_tokens 调低；model/messages/stream 由后端保护，避免误伤 challenge。',
    '{}',
    'merge',
    '{"max_tokens": 20}'
),
(
    'OpenAI Responses / 本站自检',
    'openai',
    'responses',
    '适用于本站或原生 Responses API：POST /v1/responses，默认 payload 自动带 instructions 与 input，避免 Instructions are required。',
    '{}',
    'off',
    NULL
),
(
    'OpenAI Responses 低 token 检测',
    'openai',
    'responses',
    '仍走 /v1/responses，仅把 max_output_tokens 调低；instructions/input/model/stream 由后端保护。',
    '{}',
    'merge',
    '{"max_output_tokens": 20}'
)
ON DUPLICATE KEY UPDATE id = id;
