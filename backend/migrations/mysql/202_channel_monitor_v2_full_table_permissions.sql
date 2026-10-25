-- MySQL 方言版本（翻译自 ../202_channel_monitor_v2_full_table_permissions.sql）
-- Extend privileges for all Channel Monitor V2 tables (base + rollup + config).
-- Grant to the migration/application role rather than the historical hard-coded
-- "sub2api" role so custom DATABASE_USER deployments work as expected.
--
-- 说明：PG 的一条多表 GRANT → MySQL 按表逐条 GRANT（MySQL 单条 GRANT 只能指定一个授权对象）；
--       CURRENT_USER 在 MySQL GRANT 中同样合法。执行迁移的用户即建表用户，本身已拥有
--       这些表的全部权限，此语句为语义对齐保留（幂等）。
GRANT SELECT, INSERT, UPDATE, DELETE ON channel_monitor_v2_config TO CURRENT_USER;
GRANT SELECT, INSERT, UPDATE, DELETE ON channel_monitor_v2_watermarks TO CURRENT_USER;
GRANT SELECT, INSERT, UPDATE, DELETE ON channel_monitor_v2_metrics_1m TO CURRENT_USER;
GRANT SELECT, INSERT, UPDATE, DELETE ON channel_monitor_v2_user_metrics_1m TO CURRENT_USER;
GRANT SELECT, INSERT, UPDATE, DELETE ON channel_monitor_v2_error_metrics_1m TO CURRENT_USER;
GRANT SELECT, INSERT, UPDATE, DELETE ON channel_monitor_v2_latency_histograms_1m TO CURRENT_USER;
GRANT SELECT, INSERT, UPDATE, DELETE ON channel_monitor_v2_metrics_rollup TO CURRENT_USER;
GRANT SELECT, INSERT, UPDATE, DELETE ON channel_monitor_v2_user_metrics_rollup TO CURRENT_USER;
GRANT SELECT, INSERT, UPDATE, DELETE ON channel_monitor_v2_error_metrics_rollup TO CURRENT_USER;
GRANT SELECT, INSERT, UPDATE, DELETE ON channel_monitor_v2_latency_histograms_rollup TO CURRENT_USER;
