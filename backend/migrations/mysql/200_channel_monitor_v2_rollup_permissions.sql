-- MySQL 方言版本（翻译自 ../200_channel_monitor_v2_rollup_permissions.sql）
-- Runtime aggregator reads/writes the fixed rollup caches through the role that
-- executes migrations.  Use CURRENT_USER so custom DATABASE_USER values work;
-- PostgreSQL permits this form in GRANT and it remains idempotent.
--
-- 说明：PG 的一条多表 GRANT → MySQL 按表逐条 GRANT（MySQL 单条 GRANT 只能指定一个授权对象）；
--       CURRENT_USER 在 MySQL GRANT 中同样合法。执行迁移的用户即建表用户，本身已拥有
--       这些表的全部权限，此语句为语义对齐保留（幂等）。
GRANT SELECT, INSERT, UPDATE, DELETE ON channel_monitor_v2_metrics_rollup TO CURRENT_USER;
GRANT SELECT, INSERT, UPDATE, DELETE ON channel_monitor_v2_user_metrics_rollup TO CURRENT_USER;
GRANT SELECT, INSERT, UPDATE, DELETE ON channel_monitor_v2_error_metrics_rollup TO CURRENT_USER;
GRANT SELECT, INSERT, UPDATE, DELETE ON channel_monitor_v2_latency_histograms_rollup TO CURRENT_USER;
