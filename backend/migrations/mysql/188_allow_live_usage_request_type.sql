-- MySQL 方言版本（翻译自 ../188_allow_live_usage_request_type.sql）
--
-- 说明：PG DROP CONSTRAINT（CHECK 约束）→ MySQL DROP CHECK（约束同名 usage_logs_request_type_check，
--       由 ../061 迁移创建）；CHECK 约束需 MySQL 8.0.16+ 才强制执行。
--       MySQL 不支持 DROP CHECK 的 IF EXISTS，由迁移执行器保证单次执行。
ALTER TABLE usage_logs
    DROP CHECK usage_logs_request_type_check;

ALTER TABLE usage_logs
    ADD CONSTRAINT usage_logs_request_type_check
    CHECK (request_type >= 0 AND request_type <= 5);
