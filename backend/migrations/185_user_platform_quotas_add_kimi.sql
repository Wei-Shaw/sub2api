-- 把 kimi 平台加入 user_platform_quotas.platform 的 CHECK 约束。
--
-- 背景：kimi 自 2026-07 起进入默认平台配额（default_platform_quotas /
-- auth_source_default_*_platform_quotas），但既有 CHECK 仅允许
-- anthropic/openai/gemini/antigravity/grok。自助注册时 snapshotPlatformQuotaDefaults
-- 会写入 kimi 默认配额行 → 违反 CHECK → 整个注册事务被标记 aborted。
--
-- 修复：把约束与代码平台列表（internal/domain/constants.go 的 PlatformKimi）对齐。
-- DROP ... IF EXISTS 保证可重入；新约束是旧约束的超集，存量行瞬时校验通过。
ALTER TABLE user_platform_quotas
    DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;

ALTER TABLE user_platform_quotas
    ADD CONSTRAINT user_platform_quotas_platform_check
    CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'kimi'));
