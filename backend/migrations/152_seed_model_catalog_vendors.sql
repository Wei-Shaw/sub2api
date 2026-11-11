-- 模型目录种子：OpenAI / Gemini / xAI Grok / Kimi / DeepSeek / GLM 已发布模型（截至 2026-11）。
--
-- 说明：
--   * Grok / Kimi / DeepSeek / GLM 通过 OpenAI 兼容上游接入，归入 openai 平台（API Key 账号 + 自定义 Base URL）
--   * 价格单位 USD / token；NULL 表示不覆盖，沿用 LiteLLM 动态价格或内置回退
--   * Gemini 4 与 3.6/3.8 Flash 当前为限时促销价、gpt-5.6-sol 来源价格不一致，故不写价格
--   * 价格为录入时的官方公开价，请管理员在「模型目录」页面按实际结算价核对
--   * 已存在的条目不会被覆盖（ON CONFLICT DO NOTHING）

INSERT INTO model_catalog (model_id, platform, vendor, display_name, input_price, output_price, cache_write_price, cache_read_price, context_window, max_output_tokens, released_at, sort_order)
VALUES
    ('gpt-6-astra', 'openai', 'openai', 'GPT-6 Astra', 1e-05, 5e-05, 1e-05, 1e-06, 1050000, 128000, '2026-09-03', 100),
    ('gpt-6.1-sol', 'openai', 'openai', 'GPT-6.1 Sol', 2e-06, 1e-05, 2e-06, 1e-07, 1050000, 128000, '2026-09-29', 101),
    ('gpt-6.1-terra', 'openai', 'openai', 'GPT-6.1 Terra', 1e-06, 5e-06, 1e-06, 5e-08, 1050000, 128000, '2026-10-26', 102),
    ('gpt-6-sol', 'openai', 'openai', 'GPT-6 Sol', 2e-06, 1e-05, 2e-06, 2e-07, 1050000, 128000, '2026-09-22', 103),
    ('gpt-6-luna', 'openai', 'openai', 'GPT-6 Luna', 1e-07, 5e-07, 1e-07, 1e-08, 1050000, 128000, '2026-09-22', 104),
    ('gpt-5.6-sol', 'openai', 'openai', 'GPT-5.6 Sol', NULL, NULL, NULL, NULL, 1050000, 128000, '2026-07-09', 105),
    ('gpt-5.6-terra', 'openai', 'openai', 'GPT-5.6 Terra', 2e-06, 1.2e-05, 2e-06, 2e-07, 1050000, 128000, '2026-07-09', 106),
    ('gpt-5.6-luna', 'openai', 'openai', 'GPT-5.6 Luna', 2e-07, 1.2e-06, 2e-07, 2e-08, 1050000, 128000, '2026-07-09', 107),
    ('gemini-4-carbon', 'gemini', 'google', 'Gemini 4 Carbon', NULL, NULL, NULL, NULL, 1048576, 65536, '2026-11-10', 108),
    ('gemini-4-argon', 'gemini', 'google', 'Gemini 4 Argon', NULL, NULL, NULL, NULL, 1048576, 65536, '2026-10-13', 109),
    ('gemini-3.8-flash', 'gemini', 'google', 'Gemini 3.8 Flash', NULL, NULL, NULL, NULL, 1048576, 65536, '2026-09-02', 110),
    ('gemini-3.6-flash', 'gemini', 'google', 'Gemini 3.6 Flash', NULL, NULL, NULL, NULL, 1048576, 65536, NULL, 111),
    ('gemini-3.5-flash-lite', 'gemini', 'google', 'Gemini 3.5 Flash-Lite', 3e-07, 2.5e-06, 3e-07, 3e-08, 1048576, 65536, NULL, 112),
    ('gemini-3.1-flash-lite', 'gemini', 'google', 'Gemini 3.1 Flash-Lite', 2.5e-07, 1.5e-06, 2.5e-07, 2.5e-08, 1048576, 65536, NULL, 113),
    ('gemini-3.1-pro-preview', 'gemini', 'google', 'Gemini 3.1 Pro Preview', 2e-06, 1.2e-05, 2e-06, 2e-07, 1048576, 65536, NULL, 114),
    ('grok-4.7', 'openai', 'xai', 'Grok 4.7', 2e-06, 6e-06, 2e-06, 5e-07, 500000, 0, '2026-09-21', 115),
    ('grok-4.6', 'openai', 'xai', 'Grok 4.6', 2e-06, 6e-06, 2e-06, 5e-07, 500000, 0, '2026-08-12', 116),
    ('grok-4.5', 'openai', 'xai', 'Grok 4.5', 2e-06, 6e-06, 2e-06, 3e-07, 500000, 0, NULL, 117),
    ('grok-4.3', 'openai', 'xai', 'Grok 4.3', 1.25e-06, 2.5e-06, 1.25e-06, 2e-07, 1000000, 0, NULL, 118),
    ('grok-4.20-0309-reasoning', 'openai', 'xai', 'Grok 4.20 Reasoning', 1.25e-06, 2.5e-06, 1.25e-06, 2e-07, 1000000, 0, NULL, 119),
    ('grok-4.20-0309-non-reasoning', 'openai', 'xai', 'Grok 4.20 Non-Reasoning', 1.25e-06, 2.5e-06, 1.25e-06, 2e-07, 1000000, 0, NULL, 120),
    ('grok-4.20-multi-agent-0309', 'openai', 'xai', 'Grok 4.20 Multi-Agent', 1.25e-06, 2.5e-06, 1.25e-06, 2e-07, 1000000, 0, NULL, 121),
    ('grok-build-0.1', 'openai', 'xai', 'Grok Build 0.1', 1e-06, 2e-06, 1e-06, 2e-07, 256000, 0, NULL, 122),
    ('kimi-k3.1-pro', 'openai', 'moonshot', 'Kimi K3.1 Pro', 2e-06, 1e-05, 2e-06, 2e-07, 1048576, 0, '2026-10-23', 123),
    ('kimi-k3.1-pro-highspeed', 'openai', 'moonshot', 'Kimi K3.1 Pro Highspeed', 4e-06, 2e-05, 4e-06, 2e-07, 1048576, 0, NULL, 124),
    ('kimi-k3', 'openai', 'moonshot', 'Kimi K3', 3e-06, 1.5e-05, 3e-06, 3e-07, 1048576, 0, '2026-07-16', 125),
    ('kimi-k2.7-code', 'openai', 'moonshot', 'Kimi K2.7 Code', 9.5e-07, 4e-06, 9.5e-07, 1.9e-07, 256000, 0, NULL, 126),
    ('kimi-k2.6', 'openai', 'moonshot', 'Kimi K2.6', 9.5e-07, 4e-06, 9.5e-07, 1.6e-07, 256000, 0, '2026-04-20', 127),
    ('kimi-for-coding', 'openai', 'moonshot', 'Kimi for Coding (Kimi Code)', NULL, NULL, NULL, NULL, 0, 0, NULL, 128),
    ('deepseek-flash', 'openai', 'deepseek', 'DeepSeek V4.1 Flash', 3e-07, 1.2e-06, 3e-07, 6e-09, 1000000, 384000, '2026-09-10', 129),
    ('deepseek-v4-flash', 'openai', 'deepseek', 'DeepSeek V4 Flash (alias)', 3e-07, 1.2e-06, 3e-07, 6e-09, 1000000, 384000, NULL, 130),
    ('deepseek-v4-pro', 'openai', 'deepseek', 'DeepSeek V4 Pro', 1.32e-06, 3.96e-06, 1.32e-06, 4.4e-08, 1000000, 384000, NULL, 131),
    ('glm-5.3', 'openai', 'zhipu', 'GLM-5.3', 1.4e-06, 4.4e-06, 1.4e-06, 2.6e-07, 1000000, 128000, '2026-08-18', 132),
    ('glm-5.3-flash', 'openai', 'zhipu', 'GLM-5.3 Flash', 1.5e-07, 5e-07, 1.5e-07, 3e-08, 1000000, 0, '2026-08-26', 133),
    ('glm-5.3-flashx', 'openai', 'zhipu', 'GLM-5.3 FlashX', 3.7e-07, 1.25e-06, 3.7e-07, 7.5e-08, 0, 0, NULL, 134),
    ('glm-5.2', 'openai', 'zhipu', 'GLM-5.2', 1.4e-06, 4.4e-06, 1.4e-06, 2.6e-07, 1000000, 128000, '2026-06-16', 135),
    ('glm-5.1', 'openai', 'zhipu', 'GLM-5.1', 1.4e-06, 4.4e-06, 1.4e-06, 2.6e-07, 200000, 128000, '2026-04-07', 136),
    ('glm-5', 'openai', 'zhipu', 'GLM-5', 1e-06, 3.2e-06, 1e-06, 2e-07, 200000, 128000, '2026-02-12', 137),
    ('glm-4.7', 'openai', 'zhipu', 'GLM-4.7', 6e-07, 2.2e-06, 6e-07, 1.1e-07, 200000, 128000, '2025-12-22', 138),
    ('glm-4.6', 'openai', 'zhipu', 'GLM-4.6', 6e-07, 2.2e-06, 6e-07, 1.1e-07, 200000, 128000, '2025-09-30', 139)
ON CONFLICT (platform, model_id) DO NOTHING;
