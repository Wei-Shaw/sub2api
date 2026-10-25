-- MySQL 方言版本（翻译自 ../169_batch_image_parent_batch.sql）
-- 翻译说明：
--   - ADD COLUMN IF NOT EXISTS → ADD COLUMN（IF NOT EXISTS 按迁移执行器单次执行语义去掉）。
--   - 非唯一部分索引（WHERE parent_batch_id IS NOT NULL AND parent_batch_id <> ''）→ 去掉 WHERE
--     改为普通索引（仅性能差异，语义等价）。
--   - COMMENT ON COLUMN → 整句跳过。

ALTER TABLE batch_image_jobs
    ADD COLUMN parent_batch_id VARCHAR(64) NULL;

CREATE INDEX batch_image_jobs_parent_batch_id_idx
    ON batch_image_jobs (parent_batch_id);

-- (comment skipped)
