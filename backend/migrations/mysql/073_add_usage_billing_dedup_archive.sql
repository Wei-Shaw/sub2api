-- MySQL 方言版本（翻译自 ../073_add_usage_billing_dedup_archive.sql）
-- 冷归档旧账务幂等键，缩小热表索引与清理范围，同时不丢失长期去重能力。

CREATE TABLE IF NOT EXISTS usage_billing_dedup_archive (
    request_id VARCHAR(255) NOT NULL,
    api_key_id BIGINT NOT NULL,
    request_fingerprint VARCHAR(64) NOT NULL,
    created_at DATETIME(3) NOT NULL,
    archived_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (request_id, api_key_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
