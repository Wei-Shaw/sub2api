-- MySQL 方言版本（翻译自 ../160_batch_image_provider_refs.sql）
-- 翻译说明：ADD COLUMN IF NOT EXISTS → ADD COLUMN（IF NOT EXISTS 按迁移执行器单次执行语义去掉）。

ALTER TABLE batch_image_jobs
    ADD COLUMN provider_input_ref VARCHAR(1024) NULL,
    ADD COLUMN provider_output_ref VARCHAR(1024) NULL;
