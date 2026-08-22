ALTER TABLE batch_image_jobs
    ADD COLUMN IF NOT EXISTS task_name VARCHAR(255) NOT NULL DEFAULT '';

UPDATE batch_image_jobs
SET task_name = TO_CHAR(created_at AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')
WHERE task_name = '';

CREATE INDEX IF NOT EXISTS batch_image_jobs_task_name_idx ON batch_image_jobs (task_name);

COMMENT ON COLUMN batch_image_jobs.task_name IS '用户可读的批量生图任务名称';
