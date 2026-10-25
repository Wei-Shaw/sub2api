-- MySQL 方言版本（翻译自 ../237_add_minimax_platform.sql）
-- 翻译说明：
--   - DROP CONSTRAINT IF EXISTS（CHECK 约束）→ ALTER TABLE ... DROP CHECK
--     （IF EXISTS 按迁移执行器单次执行语义去掉；与 157/226/227 号翻译同型处理，
--     约束同名重建；CHECK 约束需 MySQL 8.0.16+ 才强制执行）。
--   - PG DO 块（pg_get_constraintdef 探测 provider CHECK 是否已含 'minimax'，
--     缺失才重建）→ 按 176 号先例去掉探测分支，直接以同名重建两个 provider CHECK
--     （迁移执行器保证单次执行，且 MySQL 侧 226 号迁移建立的约束不含 minimax）。

-- 把 MiniMax 加入国产供应商平台白名单：
--   1. user_platform_quotas.platform CHECK
--   2. composite_model_routes.target_platform CHECK
--   3. channel_monitors / channel_monitor_request_templates.provider CHECK
ALTER TABLE user_platform_quotas
    DROP CHECK user_platform_quotas_platform_check;

ALTER TABLE user_platform_quotas
    ADD CONSTRAINT user_platform_quotas_platform_check
    CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok',
                        'kimi', 'zhipu', 'deepseek', 'minimax'));

ALTER TABLE composite_model_routes
    DROP CHECK composite_model_routes_target_platform_check;

ALTER TABLE composite_model_routes
    ADD CONSTRAINT composite_model_routes_target_platform_check
    CHECK (target_platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok',
                               'kimi', 'zhipu', 'deepseek', 'minimax'));

ALTER TABLE channel_monitors
    DROP CHECK channel_monitors_provider_check;

ALTER TABLE channel_monitors
    ADD CONSTRAINT channel_monitors_provider_check
    CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok',
                        'antigravity', 'kimi', 'zhipu', 'deepseek', 'minimax'));

ALTER TABLE channel_monitor_request_templates
    DROP CHECK channel_monitor_request_templates_provider_check;

ALTER TABLE channel_monitor_request_templates
    ADD CONSTRAINT channel_monitor_request_templates_provider_check
    CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok',
                        'antigravity', 'kimi', 'zhipu', 'deepseek', 'minimax'));
