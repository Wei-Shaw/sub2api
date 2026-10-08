-- 把 Ollama Cloud 加入平台白名单。
--
-- 1. user_platform_quotas.platform CHECK
-- 2. composite_model_routes.target_platform CHECK
-- 3. channel_monitors / channel_monitor_request_templates provider CHECK
--
-- 顺序：必须晚于 241_add_typesafe_platform.sql。上游 241 把前两个平台 CHECK 覆写为
-- 11 项（含 typesafe、不含 ollama_cloud），本迁移在其后重建为 12 项，即 241 的平台
-- 集合 ∪ {ollama_cloud}。两张监控表的 provider CHECK 以 238_opencode_go_platform.sql
-- 的 10 项为基准（typesafe 不是对话模型，不进 provider 白名单），并集 'ollama_cloud'。
--
-- DROP ... IF EXISTS + 幂等守卫保证可重入；新约束是旧约束的超集，存量行瞬时通过校验，
-- 因此是默认的 validated 约束。零 DML 回填：与 237/238/241 一致，只放宽约束，不写任何
-- DML。监控表的守卫以 pg_get_constraintdef 探测：约束已含 ollama_cloud 时整段跳过，避免
-- 无谓重建。
--
-- 适用 fresh 建库、空表升级与已应用 241 的环境。若库中已存在 platform='ollama_cloud'
-- 的行，说明其写入发生在更宽松的旧约束下，需在启动前停写核对；本迁移不做数据修复，
-- 也不能替代该前置处置。

ALTER TABLE user_platform_quotas
    DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;

ALTER TABLE user_platform_quotas
    ADD CONSTRAINT user_platform_quotas_platform_check
    CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok',
                        'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'typesafe',
                        'ollama_cloud'));

ALTER TABLE composite_model_routes
    DROP CONSTRAINT IF EXISTS composite_model_routes_target_platform_check;

ALTER TABLE composite_model_routes
    ADD CONSTRAINT composite_model_routes_target_platform_check
    CHECK (target_platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok',
                               'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go', 'typesafe',
                               'ollama_cloud'));

DO $$
DECLARE
    monitor_constraint_def TEXT;
    template_constraint_def TEXT;
BEGIN
    SELECT pg_get_constraintdef(c.oid)
      INTO monitor_constraint_def
      FROM pg_constraint c
      JOIN pg_class t ON t.oid = c.conrelid
     WHERE t.relname = 'channel_monitors'
       AND c.conname = 'channel_monitors_provider_check';

    IF monitor_constraint_def IS NULL OR position('ollama_cloud' IN monitor_constraint_def) = 0 THEN
        ALTER TABLE channel_monitors
            DROP CONSTRAINT IF EXISTS channel_monitors_provider_check;
        ALTER TABLE channel_monitors
            ADD CONSTRAINT channel_monitors_provider_check
            CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok',
                                'antigravity', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go',
                                'ollama_cloud'));
    END IF;

    SELECT pg_get_constraintdef(c.oid)
      INTO template_constraint_def
      FROM pg_constraint c
      JOIN pg_class t ON t.oid = c.conrelid
     WHERE t.relname = 'channel_monitor_request_templates'
       AND c.conname = 'channel_monitor_request_templates_provider_check';

    IF template_constraint_def IS NULL OR position('ollama_cloud' IN template_constraint_def) = 0 THEN
        ALTER TABLE channel_monitor_request_templates
            DROP CONSTRAINT IF EXISTS channel_monitor_request_templates_provider_check;
        ALTER TABLE channel_monitor_request_templates
            ADD CONSTRAINT channel_monitor_request_templates_provider_check
            CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok',
                                'antigravity', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go',
                                'ollama_cloud'));
    END IF;
END $$;
