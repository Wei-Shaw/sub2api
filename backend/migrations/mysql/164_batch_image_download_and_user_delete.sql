-- MySQL 方言版本（翻译自 ../164_batch_image_download_and_user_delete.sql）
-- 翻译说明：
--   - TIMESTAMPTZ → DATETIME(3)（可空）。
--   - CREATE INDEX IF NOT EXISTS → CREATE INDEX（IF NOT EXISTS 按迁移执行器单次执行语义去掉）。
--   - COMMENT ON COLUMN → 整句跳过。

ALTER TABLE batch_image_jobs
    ADD COLUMN downloaded_at DATETIME(3) NULL,
    ADD COLUMN user_deleted_at DATETIME(3) NULL;

CREATE INDEX batch_image_jobs_downloaded_at_idx ON batch_image_jobs (downloaded_at);
CREATE INDEX batch_image_jobs_user_deleted_at_idx ON batch_image_jobs (user_deleted_at);

-- (comment skipped)
-- (comment skipped)
