-- MySQL 方言版本（翻译自 ../096_payment_provider_instances.sql）
-- 翻译说明：BIGSERIAL → BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY；
--           TIMESTAMPTZ/NOW() → DATETIME(3)/CURRENT_TIMESTAMP(3)；
--           TEXT 列默认值使用表达式默认值 DEFAULT ('')（MySQL 8.0.13+）；
--           limits 为 MySQL 8 保留字（LIMIT），列名加反引号。

CREATE TABLE IF NOT EXISTS payment_provider_instances (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    provider_key VARCHAR(30) NOT NULL,
    name VARCHAR(100) NOT NULL DEFAULT '',
    config TEXT NOT NULL,
    supported_types VARCHAR(200) NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INT NOT NULL DEFAULT 0,
    `limits` TEXT NOT NULL DEFAULT (''),
    refund_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
CREATE INDEX idx_payment_provider_instances_provider_key ON payment_provider_instances(provider_key);
CREATE INDEX idx_payment_provider_instances_enabled ON payment_provider_instances(enabled);
