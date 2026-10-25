-- MySQL 方言版本（翻译自 ../042_add_usage_cleanup_tasks.sql）
-- 042_add_usage_cleanup_tasks.sql
-- 使用记录清理任务表

CREATE TABLE IF NOT EXISTS usage_cleanup_tasks (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    status VARCHAR(20) NOT NULL,
    filters JSON NOT NULL,
    created_by BIGINT NOT NULL,
    deleted_rows BIGINT NOT NULL DEFAULT 0,
    error_message TEXT NULL,
    started_at DATETIME(3) NULL,
    finished_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT fk_usage_cleanup_tasks_created_by FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_usage_cleanup_tasks_status_created_at
    ON usage_cleanup_tasks(status, created_at DESC);

CREATE INDEX idx_usage_cleanup_tasks_created_at
    ON usage_cleanup_tasks(created_at DESC);
