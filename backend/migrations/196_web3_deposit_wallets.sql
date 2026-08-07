CREATE TABLE IF NOT EXISTS web3_deposit_wallets (
    id BIGSERIAL PRIMARY KEY,
    wallet_id VARCHAR(64) NOT NULL UNIQUE,
    account_path VARCHAR(64) NOT NULL,
    xpub_fingerprint VARCHAR(64) NOT NULL,
    next_derivation_index BIGINT NOT NULL DEFAULT 0,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT web3_deposit_wallets_id_format_check
        CHECK (wallet_id ~ '^[a-z0-9_]{1,64}$'),
    CONSTRAINT web3_deposit_wallets_account_path_check
        CHECK (account_path ~ '^m/44''/60''/(0|[1-9][0-9]*)''$'),
    CONSTRAINT web3_deposit_wallets_fingerprint_check
        CHECK (xpub_fingerprint ~ '^[0-9a-f]{64}$'),
    CONSTRAINT web3_deposit_wallets_next_index_check
        CHECK (next_derivation_index >= 0 AND next_derivation_index < 2147483648),
    CONSTRAINT web3_deposit_wallets_status_check
        CHECK (status IN ('active', 'disabled'))
);
