CREATE TABLE IF NOT EXISTS web3_deposit_addresses (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    wallet_id VARCHAR(64) NOT NULL REFERENCES web3_deposit_wallets(wallet_id),
    derivation_index BIGINT NOT NULL,
    address VARCHAR(42) NOT NULL,
    normalized_address VARCHAR(42) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    allocated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    disabled_at TIMESTAMPTZ,
    last_deposit_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT web3_deposit_addresses_derivation_index_check
        CHECK (derivation_index >= 0 AND derivation_index < 2147483648),
    CONSTRAINT web3_deposit_addresses_address_check
        CHECK (address ~ '^0x[0-9a-fA-F]{40}$'),
    CONSTRAINT web3_deposit_addresses_normalized_address_check
        CHECK (normalized_address ~ '^0x[0-9a-f]{40}$'),
    CONSTRAINT web3_deposit_addresses_status_check
        CHECK (status IN ('active', 'disabled')),
    CONSTRAINT web3_deposit_addresses_user_wallet_uniq
        UNIQUE (user_id, wallet_id),
    CONSTRAINT web3_deposit_addresses_index_uniq
        UNIQUE (wallet_id, derivation_index),
    CONSTRAINT web3_deposit_addresses_address_uniq
        UNIQUE (normalized_address)
);

CREATE INDEX IF NOT EXISTS idx_web3_deposit_addresses_user_created
    ON web3_deposit_addresses (user_id, created_at DESC);
