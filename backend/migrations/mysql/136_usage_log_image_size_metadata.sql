-- MySQL 方言版本（翻译自 ../136_usage_log_image_size_metadata.sql）
-- 翻译说明：
--   - ADD COLUMN IF NOT EXISTS → ADD COLUMN（IF NOT EXISTS 按迁移执行器单次执行语义去掉）。
--   - 两个 DO 块（约束存在性检查 + ADD CONSTRAINT ... NOT VALID）：存在性分支去掉后直接执行；
--     MySQL 无 NOT VALID（跳过存量行校验），ADD CONSTRAINT 时即校验存量行
--     （等价于 PG 的 VALIDATE CONSTRAINT，语义为最终一致）。
--   - JSONB → JSON。

-- Add generated-image billing size audit metadata.
-- `image_size` remains the canonical billing tier used for cost calculation.

ALTER TABLE usage_logs
    ADD COLUMN image_input_size VARCHAR(32) NULL;

ALTER TABLE usage_logs
    ADD COLUMN image_output_size VARCHAR(32) NULL;

ALTER TABLE usage_logs
    ADD COLUMN image_size_source VARCHAR(16) NULL;

ALTER TABLE usage_logs
    ADD COLUMN image_size_breakdown JSON NULL;

ALTER TABLE usage_logs
    ADD CONSTRAINT usage_logs_image_size_source_check
    CHECK (
        image_size_source IS NULL
        OR image_size_source IN ('output', 'input', 'default', 'legacy')
    );

ALTER TABLE usage_logs
    ADD CONSTRAINT usage_logs_image_billing_size_check
    CHECK (
        image_count <= 0
        OR (
            image_size IS NOT NULL
            AND image_size IN ('1K', '2K', '4K', 'mixed')
        )
    );
