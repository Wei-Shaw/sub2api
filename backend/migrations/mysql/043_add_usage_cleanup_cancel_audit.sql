-- MySQL 方言版本（翻译自 ../043_add_usage_cleanup_cancel_audit.sql）
-- 043_add_usage_cleanup_cancel_audit.sql
-- usage_cleanup_tasks 取消任务审计字段

-- PG 内联 REFERENCES → 显式命名 FK 约束（便于后续 DROP FOREIGN KEY）
ALTER TABLE usage_cleanup_tasks
    ADD COLUMN canceled_by BIGINT NULL,
    ADD COLUMN canceled_at DATETIME(3) NULL,
    ADD CONSTRAINT fk_usage_cleanup_tasks_canceled_by FOREIGN KEY (canceled_by) REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX idx_usage_cleanup_tasks_canceled_at
    ON usage_cleanup_tasks(canceled_at DESC);
