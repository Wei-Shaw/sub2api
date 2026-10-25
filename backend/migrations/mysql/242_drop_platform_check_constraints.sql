-- MySQL 方言版本（翻译自 ../242_drop_platform_check_constraints.sql）
-- 翻译说明：DROP CONSTRAINT IF EXISTS（CHECK 约束）→ ALTER TABLE ... DROP CHECK
-- （IF EXISTS 按迁移执行器单次执行语义去掉；约束由 157/172/237/238 号翻译以同名建立）。

-- 平台白名单改由应用层的平台清单（internal/domain/platforms.go）统一校验，
-- 新增平台不再需要修改数据库约束的迁移。
-- channel_monitors / channel_monitor_request_templates 的 provider CHECK 保留不动。
ALTER TABLE user_platform_quotas
    DROP CHECK user_platform_quotas_platform_check;

ALTER TABLE composite_model_routes
    DROP CHECK composite_model_routes_target_platform_check;
