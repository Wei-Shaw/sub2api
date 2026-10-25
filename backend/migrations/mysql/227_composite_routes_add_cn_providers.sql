-- MySQL 方言版本（翻译自 ../227_composite_routes_add_cn_providers.sql）
-- 翻译说明：DROP CONSTRAINT IF EXISTS（CHECK 约束）→ ALTER TABLE ... DROP CHECK
-- （IF EXISTS 按迁移执行器单次执行语义去掉；约束由 172 迁移建表引入，同名保留，
-- 与 224 号翻译同型处理；CHECK 约束需 MySQL 8.0.16+ 才强制执行）。

-- Allow Composite model routes to target the three concrete CN providers.
ALTER TABLE composite_model_routes
    DROP CHECK composite_model_routes_target_platform_check;

ALTER TABLE composite_model_routes
    ADD CONSTRAINT composite_model_routes_target_platform_check
    CHECK (target_platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok',
                               'kimi', 'zhipu', 'deepseek'));
