-- 把 Adobe Firefly 加入平台白名单：
--   1. user_platform_quotas.platform CHECK
--   2. composite_model_routes.target_platform CHECK
--
-- 与 224/227/237 同型：DROP IF EXISTS 后重建超集约束，存量行瞬时校验通过。
--
-- 刻意不动 channel_monitors / channel_monitor_request_templates 的 provider CHECK：
-- 那两个字段是 ent enum（ent/schema/channel_monitor.go），加值需改 schema 并重跑
-- go generate ./ent；而渠道监控做的是聊天模型探活，对 Adobe 这种图像生成 API
-- 没有可用的探活语义。需要时再单独补一版迁移。

ALTER TABLE user_platform_quotas
    DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;

ALTER TABLE user_platform_quotas
    ADD CONSTRAINT user_platform_quotas_platform_check
    CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok',
                        'kimi', 'zhipu', 'deepseek', 'kiro', 'minimax', 'adobe'));

ALTER TABLE composite_model_routes
    DROP CONSTRAINT IF EXISTS composite_model_routes_target_platform_check;

ALTER TABLE composite_model_routes
    ADD CONSTRAINT composite_model_routes_target_platform_check
    CHECK (target_platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok',
                               'kimi', 'zhipu', 'deepseek', 'kiro', 'minimax', 'adobe'));
