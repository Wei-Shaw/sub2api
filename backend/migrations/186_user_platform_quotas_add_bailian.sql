-- 把 bailian 平台加入 user_platform_quotas.platform 的 CHECK 约束。
--
-- 背景：bailian（阿里百炼 DashScope）作为新平台加入
-- （internal/domain/constants.go 的 PlatformBailian），进入默认平台配额后
-- snapshotPlatformQuotaDefaults 会写入 bailian 配额行，必须先扩 CHECK，
-- 否则自助注册事务违反约束（同 157 加 grok 时的事故模式）。
-- DROP ... IF EXISTS 保证可重入；新约束是旧约束的超集，存量行瞬时校验通过。
ALTER TABLE user_platform_quotas
    DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;

ALTER TABLE user_platform_quotas
    ADD CONSTRAINT user_platform_quotas_platform_check
    CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'bailian'));
