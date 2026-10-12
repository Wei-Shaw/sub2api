-- 把 Ollama Cloud 加入渠道监控 provider 白名单（channel_monitors /
-- channel_monitor_request_templates 的 provider CHECK）。
--
-- 平台白名单（user_platform_quotas.platform、composite_model_routes.target_platform）
-- 已由 242_drop_platform_check_constraints.sql 改为应用层按平台清单校验，本迁移
-- 不得重建这两个约束。渠道监控 provider 表示已实现的探测能力，仍由数据库约束维护：
-- 以 238_opencode_go_platform.sql 的 10 项为基准并集 'ollama_cloud'（typesafe 不是
-- 对话模型，不进 provider 白名单）。
--
-- 守卫以 pg_get_constraintdef 探测：约束已含 ollama_cloud 时整段跳过，保证可重入。
-- 新约束是旧约束的超集，存量行瞬时通过校验，因此是默认的 validated 约束。零 DML。

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
