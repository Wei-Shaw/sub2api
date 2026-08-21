CREATE TABLE IF NOT EXISTS bot_sales_fulfillments (
    id BIGSERIAL PRIMARY KEY,
    idempotency_key VARCHAR(255) NOT NULL,
    external_order_code VARCHAR(255) NOT NULL,
    external_order_item_id VARCHAR(255) NOT NULL,
    request_fingerprint TEXT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'processing',
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    balance_amount DECIMAL(20, 8) NOT NULL,
    response_json TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT bot_sales_fulfillments_status_check CHECK (status IN ('processing', 'succeeded')),
    CONSTRAINT bot_sales_fulfillments_idempotency_key_key UNIQUE (idempotency_key),
    CONSTRAINT bot_sales_fulfillments_order_item_key UNIQUE (external_order_code, external_order_item_id)
);

CREATE INDEX IF NOT EXISTS bot_sales_fulfillments_user_id_idx ON bot_sales_fulfillments(user_id);
CREATE INDEX IF NOT EXISTS bot_sales_fulfillments_status_idx ON bot_sales_fulfillments(status);
