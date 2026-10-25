-- MySQL 方言版本（翻译自 ../036_scheduler_outbox.sql）

CREATE TABLE IF NOT EXISTS scheduler_outbox (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    event_type TEXT NOT NULL,
    account_id BIGINT NULL,
    group_id BIGINT NULL,
    payload JSON NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_scheduler_outbox_created_at ON scheduler_outbox (created_at);
