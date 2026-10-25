-- MySQL 方言版本（翻译自 ../093_payment_audit_logs.sql）
-- 翻译说明：BIGSERIAL → BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY；
--           TIMESTAMPTZ/NOW() → DATETIME(3)/CURRENT_TIMESTAMP(3)；
--           TEXT 列默认值使用表达式默认值 DEFAULT ('')（MySQL 8.0.13+）。

CREATE TABLE IF NOT EXISTS payment_audit_logs (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    order_id VARCHAR(64) NOT NULL,
    action VARCHAR(50) NOT NULL,
    detail TEXT NOT NULL DEFAULT (''),
    operator VARCHAR(100) NOT NULL DEFAULT 'system',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
CREATE INDEX idx_payment_audit_logs_order_id ON payment_audit_logs(order_id);
