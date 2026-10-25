-- MySQL 方言版本（翻译自 ../057_add_idempotency_records.sql）
-- 幂等记录表：用于关键写接口的请求去重与结果重放
-- 幂等执行：可重复运行（由迁移执行器保证单次执行）

CREATE TABLE IF NOT EXISTS idempotency_records (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    scope VARCHAR(128) NOT NULL,
    idempotency_key_hash VARCHAR(64) NOT NULL,
    request_fingerprint VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL,
    response_status INTEGER,
    response_body TEXT,
    error_reason VARCHAR(128),
    locked_until DATETIME(3) NULL,
    expires_at DATETIME(3) NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE UNIQUE INDEX idx_idempotency_records_scope_key
    ON idempotency_records (scope, idempotency_key_hash);

CREATE INDEX idx_idempotency_records_expires_at
    ON idempotency_records (expires_at);

CREATE INDEX idx_idempotency_records_status_locked_until
    ON idempotency_records (status, locked_until);
