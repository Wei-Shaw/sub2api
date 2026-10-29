UPDATE batch_image_jobs
SET task_name = TO_CHAR(created_at AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS')
WHERE task_name = '';

COMMENT ON COLUMN batch_image_jobs.task_name IS '用户填写的批量生图任务名称；提交时为空则默认写入当前时间';
