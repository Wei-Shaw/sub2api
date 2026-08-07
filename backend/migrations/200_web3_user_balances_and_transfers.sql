CREATE TABLE IF NOT EXISTS web3_user_balances (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    asset_key VARCHAR(64) NOT NULL,
    available_amount DECIMAL(20,8) NOT NULL DEFAULT 0,
    total_deposited DECIMAL(20,8) NOT NULL DEFAULT 0,
    total_transferred DECIMAL(20,8) NOT NULL DEFAULT 0,
    balance_version BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT web3_user_balances_id_user_uniq
        UNIQUE (id, user_id),
    CONSTRAINT web3_user_balances_user_asset_uniq
        UNIQUE (user_id, asset_key),
    CONSTRAINT web3_user_balances_asset_key_check
        CHECK (asset_key ~ '^[a-z0-9_]{1,64}$'),
    CONSTRAINT web3_user_balances_available_amount_check
        CHECK (available_amount >= 0),
    CONSTRAINT web3_user_balances_total_deposited_check
        CHECK (total_deposited >= 0),
    CONSTRAINT web3_user_balances_total_transferred_check
        CHECK (total_transferred >= 0),
    CONSTRAINT web3_user_balances_version_check
        CHECK (balance_version >= 0),
    CONSTRAINT web3_user_balances_reconciliation_check
        CHECK (available_amount = total_deposited - total_transferred)
);

CREATE TABLE IF NOT EXISTS web3_balance_transfers (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    web3_balance_id BIGINT NOT NULL,
    amount DECIMAL(20,8) NOT NULL,
    web3_balance_before DECIMAL(20,8) NOT NULL,
    web3_balance_after DECIMAL(20,8) NOT NULL,
    user_balance_before DECIMAL(20,8) NOT NULL,
    user_balance_after DECIMAL(20,8) NOT NULL,
    idempotency_key VARCHAR(180) NOT NULL UNIQUE,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT web3_balance_transfers_balance_user_fk
        FOREIGN KEY (web3_balance_id, user_id)
        REFERENCES web3_user_balances(id, user_id),
    CONSTRAINT web3_balance_transfers_amount_check
        CHECK (amount > 0),
    CONSTRAINT web3_balance_transfers_web3_before_check
        CHECK (web3_balance_before >= 0),
    CONSTRAINT web3_balance_transfers_web3_after_check
        CHECK (web3_balance_after >= 0),
    CONSTRAINT web3_balance_transfers_web3_reconciliation_check
        CHECK (web3_balance_after = web3_balance_before - amount),
    CONSTRAINT web3_balance_transfers_user_reconciliation_check
        CHECK (user_balance_after = user_balance_before + amount)
);

CREATE INDEX IF NOT EXISTS idx_web3_balance_transfers_user_history
    ON web3_balance_transfers (user_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_web3_balance_transfers_balance_history
    ON web3_balance_transfers (web3_balance_id, created_at DESC, id DESC);
