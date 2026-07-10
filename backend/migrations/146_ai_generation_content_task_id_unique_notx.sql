-- Video capture rows use task_id as their durable idempotency key because
-- request_id/api_key_id are chat gateway concepts and remain empty for video.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_ai_generation_content_task_id
    ON ai_generation_content(task_id)
    WHERE task_id IS NOT NULL;
