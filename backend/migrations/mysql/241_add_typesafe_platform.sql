-- MySQL 方言版本（翻译自 ../241_add_typesafe_platform.sql）
-- 翻译说明：DROP CONSTRAINT IF EXISTS（CHECK 约束）→ ALTER TABLE ... DROP CHECK
-- （IF EXISTS 按迁移执行器单次执行语义去掉；与 157/227/237/238 号翻译同型处理，
-- 约束同名重建；CHECK 约束需 MySQL 8.0.16+ 才强制执行）。

-- Add TypeSafe (Jev System One) as a first-class platform.
--
-- TypeSafe 不是对话模型，不进入渠道监控 provider，因此 channel_monitors /
-- channel_monitor_request_templates 的约束保持不变。
-- 新约束是 238 的超集，必须同时保留 MiniMax 与 OpenCode GO。
ALTER TABLE user_platform_quotas
    DROP CHECK user_platform_quotas_platform_check;

ALTER TABLE user_platform_quotas
    ADD CONSTRAINT user_platform_quotas_platform_check
    CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok',
                        'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'typesafe'));

ALTER TABLE composite_model_routes
    DROP CHECK composite_model_routes_target_platform_check;

ALTER TABLE composite_model_routes
    ADD CONSTRAINT composite_model_routes_target_platform_check
    CHECK (target_platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok',
                               'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'typesafe'));
