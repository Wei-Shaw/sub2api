-- MySQL 方言版本（翻译自 ../166_batch_image_task_name.sql）
-- 翻译说明：
--   - ADD COLUMN IF NOT EXISTS / CREATE INDEX IF NOT EXISTS → 去掉 IF NOT EXISTS（迁移执行器保证单次执行）。
--   - TO_CHAR(created_at AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')
--     → DATE_FORMAT(CONVERT_TZ(created_at, '+00:00', '+08:00'), '%Y-%m-%d %H:%i:%s')
--     （PG timestamptz AT TIME ZONE 语义为 UTC → 上海本地时间）。
--   - COMMENT ON COLUMN → 整句跳过。

ALTER TABLE batch_image_jobs
    ADD COLUMN task_name VARCHAR(255) NOT NULL DEFAULT '';

UPDATE batch_image_jobs
SET task_name = DATE_FORMAT(CONVERT_TZ(created_at, '+00:00', '+08:00'), '%Y-%m-%d %H:%i:%s')
WHERE task_name = '';

CREATE INDEX batch_image_jobs_task_name_idx ON batch_image_jobs (task_name);

-- (comment skipped)
