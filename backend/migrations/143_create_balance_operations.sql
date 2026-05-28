CREATE TABLE IF NOT EXISTS balance_operations (
    id BIGSERIAL PRIMARY KEY,
    external_op_id VARCHAR(128) NOT NULL UNIQUE,
    user_id BIGINT NOT NULL,
    op_type VARCHAR(8) NOT NULL,
    amount DECIMAL(20, 8) NOT NULL,
    balance_before DECIMAL(20, 8) NOT NULL DEFAULT 0,
    balance_after DECIMAL(20, 8) NOT NULL DEFAULT 0,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    failure_reason VARCHAR(255),
    note VARCHAR(255),
    request_payload JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_balance_operations_user_id_created_at
    ON balance_operations (user_id, created_at);

CREATE INDEX IF NOT EXISTS idx_balance_operations_status
    ON balance_operations (status);
