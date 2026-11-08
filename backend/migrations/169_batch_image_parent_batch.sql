ALTER TABLE batch_image_jobs
    ADD COLUMN IF NOT EXISTS parent_batch_id VARCHAR(64);

CREATE INDEX IF NOT EXISTS batch_image_jobs_parent_batch_id_idx
    ON batch_image_jobs (parent_batch_id)
    WHERE parent_batch_id IS NOT NULL AND parent_batch_id <> '';

COMMENT ON COLUMN batch_image_jobs.parent_batch_id IS '父批量生图任务 ID；失败项重试等子任务挂在主任务下展示';
