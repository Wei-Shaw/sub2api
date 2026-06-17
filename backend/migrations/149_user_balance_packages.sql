CREATE TABLE IF NOT EXISTS user_balance_packages (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    redeem_code_id BIGINT NULL REFERENCES redeem_codes(id) ON DELETE SET NULL,
    amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    remaining_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT user_balance_packages_amount_nonnegative CHECK (amount >= 0),
    CONSTRAINT user_balance_packages_remaining_nonnegative CHECK (remaining_amount >= 0),
    CONSTRAINT user_balance_packages_remaining_lte_amount CHECK (remaining_amount <= amount)
);

CREATE INDEX IF NOT EXISTS idx_user_balance_packages_user_active_expiry
    ON user_balance_packages (user_id, status, expires_at, id)
    WHERE remaining_amount > 0;

CREATE INDEX IF NOT EXISTS idx_user_balance_packages_redeem_code
    ON user_balance_packages (redeem_code_id);
