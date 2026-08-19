-- 把 kimi/zhipu/deepseek 平台加入 user_platform_quotas.platform 的 CHECK 约束。
--
-- 背景：国产供应商进入 AllowedQuotaPlatforms（internal/service/domain_constants.go），
-- 注册时 GetDefaultPlatformQuotas 会为全部 8 平台预填充默认配额行，但 157 号迁移的
-- CHECK 仍只允许 5 平台。BulkInsertInitial 是单条多行 INSERT，任一违约行会中止整条
-- 语句 → 注册路径 fail-open 吞错 → 新用户拿到零条配额记录（含原有 5 平台，缺失配额
-- 行 = 无限额）。与 157 头注释记载的 grok 同型事故一致。
--
-- 修复：把约束与代码平台列表（PlatformKimi/PlatformZhipu/PlatformDeepseek）对齐。
-- DROP ... IF EXISTS 保证可重入；新约束是旧约束的超集，存量行（仅 5 平台）瞬时校验通过。
ALTER TABLE user_platform_quotas
    DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;

ALTER TABLE user_platform_quotas
    ADD CONSTRAINT user_platform_quotas_platform_check
    CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok',
                        'kimi', 'zhipu', 'deepseek'));
