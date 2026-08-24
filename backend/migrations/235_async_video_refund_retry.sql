-- Persist async-video refund retries so failures survive process restarts.
ALTER TABLE async_video_tasks
    ADD COLUMN IF NOT EXISTS billing_type SMALLINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS refund_status VARCHAR(16) NOT NULL DEFAULT 'none',
    ADD COLUMN IF NOT EXISTS refund_attempts INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS refund_next_retry_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS refund_error VARCHAR(512);

CREATE INDEX IF NOT EXISTS async_video_tasks_refund_retry_idx
    ON async_video_tasks (refund_status, refund_next_retry_at);
