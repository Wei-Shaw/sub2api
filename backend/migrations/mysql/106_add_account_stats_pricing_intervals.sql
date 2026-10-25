-- MySQL 方言版本（翻译自 ../106_add_account_stats_pricing_intervals.sql）
-- 翻译说明：BIGSERIAL → BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY；
--           TIMESTAMPTZ/NOW() → DATETIME(3)/CURRENT_TIMESTAMP(3)；
--           NUMERIC → DECIMAL；内联 REFERENCES → 显式命名外键（fk_cas_*，
--           与 ../mysql/101_add_account_stats_pricing.sql 的命名风格一致）。

-- Add intervals table for account stats pricing rules (mirrors channel_pricing_intervals).
CREATE TABLE IF NOT EXISTS channel_account_stats_pricing_intervals (
    id                BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    pricing_id        BIGINT NOT NULL,
    min_tokens        INT NOT NULL DEFAULT 0,
    max_tokens        INT,
    tier_label        VARCHAR(50),
    input_price       DECIMAL(20,12),
    output_price      DECIMAL(20,12),
    cache_write_price DECIMAL(20,12),
    cache_read_price  DECIMAL(20,12),
    per_request_price DECIMAL(20,12),
    sort_order        INT NOT NULL DEFAULT 0,
    created_at        DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at        DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    CONSTRAINT fk_cas_pricing_intervals_pricing_id
        FOREIGN KEY (pricing_id) REFERENCES channel_account_stats_model_pricing(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE INDEX idx_account_stats_pricing_intervals_pricing_id
    ON channel_account_stats_pricing_intervals (pricing_id);
