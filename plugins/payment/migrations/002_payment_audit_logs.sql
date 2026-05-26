CREATE TABLE IF NOT EXISTS payment_audit_logs (
    id BIGSERIAL PRIMARY KEY,
    order_id VARCHAR(64) NOT NULL,
    action VARCHAR(50) NOT NULL,
    detail TEXT NOT NULL DEFAULT '',
    operator VARCHAR(100) NOT NULL DEFAULT 'system',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_payment_audit_logs_order_id ON payment_audit_logs(order_id);
