-- Migration: 239_fork_platform_constraints_superset
-- 把四个平台 CHECK 约束统一收敛为本 fork 的全集。
--
-- 背景：从官方镜像切到本 fork 的库，已按官方版本应用了 157/237/238_opencode_go
-- （约束里没有 kiro/adobe），而 fork 独有的 227/229/238_add_adobe 尚未应用。
-- 后者在这类库上是乱序补跑的，且都是 DROP + 重建、列表里缺 minimax/opencode_go，
-- 跑完会把约束收窄。约束缺平台时，注册快照 snapshotPlatformQuotaDefaults 的
-- 多行 INSERT 整条违约 → fail-open → 新用户拿到零条配额记录（缺失 = 无限额）。
--
-- 本迁移排在所有平台迁移之后，无论前面按什么顺序应用，结果都与代码对齐：
--   - user_platform_quotas / composite_model_routes：service.AllowedQuotaPlatforms 全部 12 项
--   - channel_monitors / channel_monitor_request_templates：ent enum 11 项（不含 adobe，见 238_add_adobe_platform）
-- 按 fork 顺序升级的库上约束本来就是全集，本迁移等价于 no-op。
-- DROP ... IF EXISTS 保证可重入；新约束是所有历史约束的超集，存量行瞬时校验通过。

ALTER TABLE user_platform_quotas
    DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;

ALTER TABLE user_platform_quotas
    ADD CONSTRAINT user_platform_quotas_platform_check
    CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok',
                        'kimi', 'zhipu', 'deepseek', 'kiro', 'minimax', 'adobe', 'opencode_go'));

ALTER TABLE composite_model_routes
    DROP CONSTRAINT IF EXISTS composite_model_routes_target_platform_check;

ALTER TABLE composite_model_routes
    ADD CONSTRAINT composite_model_routes_target_platform_check
    CHECK (target_platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'grok',
                               'kimi', 'zhipu', 'deepseek', 'kiro', 'minimax', 'adobe', 'opencode_go'));

ALTER TABLE channel_monitors
    DROP CONSTRAINT IF EXISTS channel_monitors_provider_check;

ALTER TABLE channel_monitors
    ADD CONSTRAINT channel_monitors_provider_check
    CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok',
                        'antigravity', 'kiro', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go'));

ALTER TABLE channel_monitor_request_templates
    DROP CONSTRAINT IF EXISTS channel_monitor_request_templates_provider_check;

ALTER TABLE channel_monitor_request_templates
    ADD CONSTRAINT channel_monitor_request_templates_provider_check
    CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok',
                        'antigravity', 'kiro', 'kimi', 'zhipu', 'deepseek', 'minimax', 'opencode_go'));
