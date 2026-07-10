CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_video_usage_logs_video_task_id
ON video_usage_logs (video_task_id);
