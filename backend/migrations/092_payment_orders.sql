CREATE TABLE IF NOT EXISTS payment_orders (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    user_email VARCHAR(255) NOT NULL DEFAULT '',
    user_name VARCHAR(100) NOT NULL DEFAULT '',
    user_notes TEXT,
    amount DECIMAL(20,2) NOT NULL,
    pay_amount DECIMAL(20,2) NOT NULL,
    fee_rate DECIMAL(10,4) NOT NULL DEFAULT 0,
    recharge_code VARCHAR(64) NOT NULL DEFAULT '',
    payment_type VARCHAR(30) NOT NULL DEFAULT '',
    payment_trade_no VARCHAR(128) NOT NULL DEFAULT '',
    pay_url TEXT,
    qr_code TEXT,
    qr_code_img TEXT,
    order_type VARCHAR(20) NOT NULL DEFAULT 'balance',
    plan_id BIGINT,
    subscription_group_id BIGINT,
    subscription_days INT,
    provider_instance_id VARCHAR(64),
    status VARCHAR(30) NOT NULL DEFAULT 'PENDING',
    refund_amount DECIMAL(20,2) NOT NULL DEFAULT 0,
    refund_reason TEXT,
    refund_at TIMESTAMPTZ,
    force_refund BOOLEAN NOT NULL DEFAULT FALSE,
    refund_requested_at TIMESTAMPTZ,
    refund_request_reason TEXT,
    refund_requested_by VARCHAR(20),
    expires_at TIMESTAMPTZ NOT NULL,
    paid_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ,
    failed_reason TEXT,
    client_ip VARCHAR(50) NOT NULL DEFAULT '',
    src_host VARCHAR(255) NOT NULL DEFAULT '',
    src_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- Indexes
CREATE INDEX IF NOT EXISTS idx_payment_orders_user_id ON payment_orders(user_id);
CREATE INDEX IF NOT EXISTS idx_payment_orders_status ON payment_orders(status);
CREATE INDEX IF NOT EXISTS idx_payment_orders_expires_at ON payment_orders(expires_at);
CREATE INDEX IF NOT EXISTS idx_payment_orders_created_at ON payment_orders(created_at);
CREATE INDEX IF NOT EXISTS idx_payment_orders_paid_at ON payment_orders(paid_at);
CREATE INDEX IF NOT EXISTS idx_payment_orders_type_paid ON payment_orders(payment_type, paid_at);
CREATE INDEX IF NOT EXISTS idx_payment_orders_order_type ON payment_orders(order_type);
