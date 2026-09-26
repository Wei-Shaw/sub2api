-- Add TypeSafe AI (Jev 判断题服务) as a first-class platform.
--
-- 只放宽 user_platform_quotas.platform CHECK：typesafe 需要 platform 级配额。
-- composite_model_routes.target_platform 与 channel_monitors /
-- channel_monitor_request_templates.provider 本阶段刻意不动——typesafe 的
-- composite 路由与渠道监控属于后续需求。
--
-- 版本顺序无关：239_ollama_cloud_platform.sql 不在本分支上但会先于本迁移在
-- integration 应用，因此新约束写成「main 与 integration 已知平台并集 + typesafe」
-- （含 ollama_cloud），无论 239/240 谁先落地都仍是旧约束的严格超集。
--
-- DROP ... IF EXISTS + 无条件 ADD 保证可重入（自研 runner 会把文件中全部 SQL
-- 当作语句执行，不解析 goose Up/Down 段）。零 DML 回填：与 157/224/237/238/239
-- 一致，只放宽约束，不写任何 DML。

ALTER TABLE user_platform_quotas
    DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;

ALTER TABLE user_platform_quotas
    ADD CONSTRAINT user_platform_quotas_platform_check
    CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok',
                        'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go',
                        'ollama_cloud', 'typesafe'));
