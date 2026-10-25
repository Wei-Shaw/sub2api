-- MySQL 方言版本（翻译自 ../035_usage_logs_partitioning.sql）
--
-- 翻译决策：
--   - PG 版检查 pg_partitioned_table 并在 usage_logs 已分区时创建上月/当月/下月分区。
--     MySQL 版按项目约定不对 usage_logs 启用分区（普通 InnoDB 表），
--     因此整个 DO 块省略，仅保留占位语句（迁移执行器要求文件非空）。
SELECT 1;
