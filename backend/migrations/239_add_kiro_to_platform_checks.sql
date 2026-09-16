-- Migration: 239_add_kiro_to_platform_checks
-- 把 kiro 重新加回 user_platform_quotas.platform 的 CHECK 约束。
--
-- 背景：与 224 号那次事故同型，这次的肇事者是上游的 237/238。
-- 上游 237_add_minimax_platform.sql 与 238_opencode_go_platform.sql 用
-- DROP CONSTRAINT + 重建全量列表的写法加 minimax / opencode_go，
-- 重建时抄的是上游自己的 10 平台列表，不含本 fork 的 kiro，
-- 相当于又把 145 / 227 的成果回退了一次。
--
-- 后果（见 227 号迁移的注释）：service.AllowedQuotaPlatforms 含 PlatformKiro，
-- GetDefaultPlatformQuotas 返回全部 11 个平台，注册时 snapshotPlatformQuotaDefaults
-- 走单条多行 INSERT（BulkInsertInitial），platform='kiro' 那一行违约会中止整条语句
-- → 快照 fail-open 仅 warn log → 新用户拿到零条配额记录（缺失配额行 = 无限额）。
-- 管理端为用户设置 kiro 配额同样直接失败。
--
-- 修复：把约束重建为 service.AllowedQuotaPlatforms 的全集（11 项，与
-- ent/schema/user_platform_quota.go 的 Validate 一致）。
-- composite_model_routes 的约束**不动**：本 fork 与后端 isConcreteRequestPlatform
-- 保持一致，kiro 不是 composite 路由目标。
-- DROP ... IF EXISTS 保证可重入；新约束是 238 的超集，存量行瞬时校验通过。

ALTER TABLE user_platform_quotas
    DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;

ALTER TABLE user_platform_quotas
    ADD CONSTRAINT user_platform_quotas_platform_check
    CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'kiro',
                        'grok', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go'));
