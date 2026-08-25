-- 移除平台枚举 CHECK 约束，平台合法性收敛到应用层校验。
--
-- 背景：user_platform_quotas.platform 与 composite_model_routes.target_platform
-- 的 CHECK 约束把平台列表固化在数据库层，每次新增供应商都必须伴随一次约束迁移，
-- 且代码列表与约束一旦脱节就是注册路径事故——157 号迁移的 CHECK 曾导致新用户
-- 注册拿不到配额行（grok），224 号迁移为 kimi/zhipu/deepseek 又重演了一次同型修复。
--
-- 平台集合的权威来源已是 CN 注册表派生的应用层校验：
--   - user_platform_quotas：service.IsAllowedQuotaPlatform
--     （setting_update.go / admin user_handler.go 全部写入入口均已覆盖）；
--   - composite_model_routes：isConcreteRequestPlatform
--     （admin_group.go 路由写入入口覆盖）。
-- 数据库层不再重复枚举，新增供应商不再需要约束迁移。
--
-- DROP ... IF EXISTS 保证可重入；移除约束对存量行无影响。
ALTER TABLE user_platform_quotas
    DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;

ALTER TABLE composite_model_routes
    DROP CONSTRAINT IF EXISTS composite_model_routes_target_platform_check;
