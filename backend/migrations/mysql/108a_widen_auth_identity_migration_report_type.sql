-- MySQL 方言版本（翻译自 ../108a_widen_auth_identity_migration_report_type.sql）
--
-- 翻译说明：
--   - DO $$ 块（仅当 report_type 列宽 < 80 时才加宽）→ 去掉 information_schema
--     条件分支，直接执行加宽：迁移执行器保证单次执行，且本目录 108 建表即为
--     VARCHAR(40)，MODIFY 为幂等操作，无条件执行结果与条件执行一致。
--   - ALTER COLUMN ... TYPE VARCHAR(80) → MODIFY COLUMN 完整列定义
--     （回溯 108 的原始定义补全 NOT NULL；report_type 不带默认值）。

ALTER TABLE auth_identity_migration_reports
    MODIFY COLUMN report_type VARCHAR(80) NOT NULL;
