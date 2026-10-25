-- MySQL 方言版本（翻译自 ../071_add_usage_billing_dedup.sql）
-- 窄表账务幂等键：将“是否已扣费”从 usage_logs 解耦出来
-- 幂等执行：可重复运行

CREATE TABLE IF NOT EXISTS usage_billing_dedup (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    request_id VARCHAR(255) NOT NULL,
    api_key_id BIGINT NOT NULL,
    request_fingerprint VARCHAR(64) NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE UNIQUE INDEX idx_usage_billing_dedup_request_api_key
    ON usage_billing_dedup (request_id, api_key_id);
