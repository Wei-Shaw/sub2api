-- 043_add_usage_cleanup_cancel_audit.sql
-- usage_cleanup_tasks 取消任务审计字段

ALTER TABLE usage_cleanup_tasks
    ADD COLUMN IF NOT EXISTS canceled_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS canceled_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_usage_cleanup_tasks_canceled_at
    ON usage_cleanup_tasks(canceled_at DESC);

