-- +migrate Up
-- 删除原有的约束
ALTER TABLE usage_logs DROP CONSTRAINT IF EXISTS usage_logs_image_size_source_check;

-- 创建新的约束，包含 output_decoded 值
ALTER TABLE usage_logs
    ADD CONSTRAINT usage_logs_image_size_source_check
    CHECK (
        image_size_source IS NULL
        OR image_size_source IN ('output', 'input', 'default', 'legacy', 'output_decoded')
    ) NOT VALID;