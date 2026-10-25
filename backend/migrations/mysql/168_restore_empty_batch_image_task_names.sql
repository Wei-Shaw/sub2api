-- MySQL 方言版本（翻译自 ../168_restore_empty_batch_image_task_names.sql）
-- 翻译说明：TO_CHAR(created_at AT TIME ZONE 'Asia/Shanghai', ...)
--           → DATE_FORMAT(CONVERT_TZ(created_at, '+00:00', '+08:00'), '%Y-%m-%d %H:%i:%s')。
--           COMMENT ON COLUMN → 整句跳过。

UPDATE batch_image_jobs
SET task_name = DATE_FORMAT(CONVERT_TZ(created_at, '+00:00', '+08:00'), '%Y-%m-%d %H:%i:%s')
WHERE task_name = '';

-- (comment skipped)
