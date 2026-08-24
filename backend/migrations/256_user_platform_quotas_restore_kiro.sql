-- Migration: 256_user_platform_quotas_restore_kiro (ported from nianzs 227; renumbered for this fork sequence)
-- 把 kiro 平台重新加入 user_platform_quotas.platform 的 CHECK 约束。
--
-- 背景：kiro 早在 145 号迁移就已进入该约束，157 号加 grok 时也一并保留。
-- 但 224 号迁移加 kimi/zhipu/deepseek 时采用 DROP + 重建的写法，重建的平台列表
-- 里漏掉了 kiro（anthropic/openai/gemini/antigravity/grok/kimi/zhipu/deepseek，共 8 个），
-- 相当于把 145 的成果回退了。
--
-- 后果与 145/157/224 记载的事故同型：AllowedQuotaPlatforms
-- （internal/service/domain_constants.go）含 PlatformKiro，GetDefaultPlatformQuotas
-- 永远返回全部 9 平台，注册时 snapshotPlatformQuotaDefaults 走单条多行 INSERT
-- （BulkInsertInitial），platform='kiro' 那一行违约会中止整条语句 → 快照 fail-open
-- 仅 warn log → 新用户拿到零条配额记录（含原有 8 平台，缺失配额行 = 无限额）。
-- 管理端为用户设置 kiro 配额同样直接失败。
--
-- 修复：把约束与代码平台列表对齐（service.AllowedQuotaPlatforms 9 项，
-- 与 ent/schema/user_platform_quota.go 的 Validate 一致）。
-- DROP ... IF EXISTS 保证可重入；新约束是旧约束的超集，存量行瞬时校验通过。
ALTER TABLE user_platform_quotas
    DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;

ALTER TABLE user_platform_quotas
    ADD CONSTRAINT user_platform_quotas_platform_check
    CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'kiro',
                        'grok', 'kimi', 'zhipu', 'deepseek'));
