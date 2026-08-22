UPDATE batch_image_jobs
SET task_name = ''
WHERE task_name = TO_CHAR(created_at AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS');

COMMENT ON COLUMN batch_image_jobs.task_name IS '用户填写的批量生图任务名称；为空时用户侧显示未填写';
