-- MySQL 方言版本（翻译自 ../095_subscription_plans.sql）
-- 翻译说明：BIGSERIAL → BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY；
--           TIMESTAMPTZ/NOW() → DATETIME(3)/CURRENT_TIMESTAMP(3)；
--           TEXT 列默认值使用表达式默认值 DEFAULT ('')（MySQL 8.0.13+）。

CREATE TABLE IF NOT EXISTS subscription_plans (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    group_id BIGINT NOT NULL,
    name VARCHAR(100) NOT NULL,
    description TEXT NOT NULL DEFAULT (''),
    price DECIMAL(20,2) NOT NULL,
    original_price DECIMAL(20,2),
    validity_days INT NOT NULL DEFAULT 30,
    validity_unit VARCHAR(10) NOT NULL DEFAULT 'day',
    features TEXT NOT NULL DEFAULT (''),
    product_name VARCHAR(100) NOT NULL DEFAULT '',
    for_sale BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INT NOT NULL DEFAULT 0,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
CREATE INDEX idx_subscription_plans_group_id ON subscription_plans(group_id);
CREATE INDEX idx_subscription_plans_for_sale ON subscription_plans(for_sale);
