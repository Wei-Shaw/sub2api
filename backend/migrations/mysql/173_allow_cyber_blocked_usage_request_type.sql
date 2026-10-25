-- MySQL 方言版本（翻译自 ../173_allow_cyber_blocked_usage_request_type.sql）
-- 翻译说明：
--   - DROP CONSTRAINT IF EXISTS（CHECK 约束）→ ALTER TABLE ... DROP CHECK（约束同名
--     usage_logs_request_type_check，由 061 迁移创建；IF EXISTS 按迁移执行器单次执行语义去掉）。
--   - NOT VALID → MySQL 不支持，去掉；ADD CONSTRAINT 时即校验存量行。

-- Cyber-policy blocks are recorded as request_type=4 so they remain visible in
-- usage audits without being confused with legacy request_type=0 rows.
ALTER TABLE usage_logs
    DROP CHECK usage_logs_request_type_check;

ALTER TABLE usage_logs
    ADD CONSTRAINT usage_logs_request_type_check
    CHECK (request_type IN (0, 1, 2, 3, 4));
