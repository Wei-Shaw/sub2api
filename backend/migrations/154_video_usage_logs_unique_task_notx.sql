CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_video_usage_logs_video_task_id
ON video_usage_logs (video_task_id);
