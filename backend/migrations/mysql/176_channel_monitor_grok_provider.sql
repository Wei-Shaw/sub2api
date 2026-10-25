-- MySQL 方言版本（翻译自 ../176_channel_monitor_grok_provider.sql）
-- 翻译说明：
--   - PG DO 块（读 pg_get_constraintdef 检查约束定义是否已含 'grok'，缺失才重建）→
--     迁移执行器保证单次执行，存在性分支去掉，直接重建两个 CHECK 约束。
--   - DROP CONSTRAINT（CHECK 约束）→ ALTER TABLE ... DROP CHECK；原约束由 125/128 迁移
--     以同名创建（provider IN ('openai','anthropic','gemini')），此处以同名 ADD CONSTRAINT
--     替换为包含 grok 的版本。CHECK 约束需 MySQL 8.0.16+ 才强制执行。
--   - position('grok' IN def) 探测逻辑无对应 MySQL 等价物，随存在性分支一并省略。

-- Migration: 176_channel_monitor_grok_provider
-- Allow Grok as a channel-monitor provider. Grok checks use the existing
-- OpenAI-compatible chat completions protocol with model grok-4.5 by default.

ALTER TABLE channel_monitors
    DROP CHECK channel_monitors_provider_check;

ALTER TABLE channel_monitors
    ADD CONSTRAINT channel_monitors_provider_check
    CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok'));

ALTER TABLE channel_monitor_request_templates
    DROP CHECK channel_monitor_request_templates_provider_check;

ALTER TABLE channel_monitor_request_templates
    ADD CONSTRAINT channel_monitor_request_templates_provider_check
    CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok'));
