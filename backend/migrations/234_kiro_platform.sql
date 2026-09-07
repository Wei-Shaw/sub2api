-- 把 kiro 平台加入 user_platform_quotas 与 composite_model_routes 的 CHECK 约束。
--
-- 背景：kiro 进入 AllowedQuotaPlatforms（internal/service/domain_constants.go）后，
-- 注册时 GetDefaultPlatformQuotas 会为全部 9 个平台预填充默认配额行。若 CHECK 仍只
-- 允许 8 个平台，BulkInsertInitial 的单条多行 INSERT 会因一行违约而整条中止 →
-- 注册路径 fail-open 吞错 → 新用户拿到零条配额记录（含原有 8 平台，缺失配额行 =
-- 无限额）。与 157（grok）、224（国产供应商）两号迁移记载的事故同型。
--
-- 修复：把约束与代码平台列表对齐。DROP ... IF EXISTS 保证可重入；
-- 新约束是旧约束的超集，存量行瞬时校验通过。
ALTER TABLE user_platform_quotas
    DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;

ALTER TABLE user_platform_quotas
    ADD CONSTRAINT user_platform_quotas_platform_check
    CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok',
                        'kimi', 'zhipu', 'deepseek', 'kiro'));

-- Composite 分组需要能把模型路由到 kiro 账号。
ALTER TABLE composite_model_routes
    DROP CONSTRAINT IF EXISTS composite_model_routes_target_platform_check;

ALTER TABLE composite_model_routes
    ADD CONSTRAINT composite_model_routes_target_platform_check
    CHECK (target_platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok',
                               'kimi', 'zhipu', 'deepseek', 'kiro'));
