CREATE TABLE IF NOT EXISTS balance_transfers (
    id BIGSERIAL PRIMARY KEY,
    external_id VARCHAR(128) NOT NULL,
    from_user_id BIGINT NOT NULL REFERENCES users(id),
    to_user_id BIGINT NOT NULL REFERENCES users(id),
    amount DECIMAL(20, 8) NOT NULL,
    reason VARCHAR(64) NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    from_balance_after DECIMAL(20, 8) NOT NULL,
    to_balance_after DECIMAL(20, 8) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_balance_transfers_amount_positive CHECK (amount > 0),
    CONSTRAINT chk_balance_transfers_distinct_users CHECK (from_user_id <> to_user_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_balance_transfers_external_id
    ON balance_transfers (external_id);

CREATE INDEX IF NOT EXISTS idx_balance_transfers_from_user_created_at
    ON balance_transfers (from_user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_balance_transfers_to_user_created_at
    ON balance_transfers (to_user_id, created_at DESC);
