-- MySQL 方言版本（翻译自 ../121_auth_identity_migration_report_type_widen.sql）
-- ALTER COLUMN x TYPE → MODIFY COLUMN <完整列定义>；
-- 回溯原始定义（../108_auth_identity_foundation_core.sql：report_type VARCHAR(40) NOT NULL，无默认值），
-- 补全 NOT NULL，仅放宽长度到 VARCHAR(80)。

ALTER TABLE auth_identity_migration_reports
    MODIFY COLUMN report_type VARCHAR(80) NOT NULL;
