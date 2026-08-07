CREATE TABLE IF NOT EXISTS web3_deposits (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    deposit_address_id BIGINT NOT NULL REFERENCES web3_deposit_addresses(id),
    chain_id BIGINT NOT NULL,
    token_contract VARCHAR(42) NOT NULL,
    tx_hash VARCHAR(66) NOT NULL,
    log_index BIGINT NOT NULL,
    block_number BIGINT NOT NULL,
    block_hash VARCHAR(66) NOT NULL,
    from_address VARCHAR(42) NOT NULL,
    to_address VARCHAR(42) NOT NULL,
    raw_amount NUMERIC(78,0) NOT NULL,
    token_decimals SMALLINT NOT NULL,
    token_amount NUMERIC(38,18) NOT NULL,
    credited_amount DECIMAL(20,8),
    status VARCHAR(32) NOT NULL DEFAULT 'detected',
    review_reason TEXT,
    failure_reason TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,
    next_retry_at TIMESTAMPTZ,
    detected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finalized_at TIMESTAMPTZ,
    credited_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT web3_deposits_chain_id_check
        CHECK (chain_id > 0),
    CONSTRAINT web3_deposits_token_contract_check
        CHECK (token_contract ~ '^0x[0-9a-f]{40}$'),
    CONSTRAINT web3_deposits_tx_hash_check
        CHECK (tx_hash ~ '^0x[0-9a-f]{64}$'),
    CONSTRAINT web3_deposits_log_index_check
        CHECK (log_index >= 0),
    CONSTRAINT web3_deposits_block_number_check
        CHECK (block_number >= 0),
    CONSTRAINT web3_deposits_block_hash_check
        CHECK (block_hash ~ '^0x[0-9a-f]{64}$'),
    CONSTRAINT web3_deposits_from_address_check
        CHECK (from_address ~ '^0x[0-9a-f]{40}$'),
    CONSTRAINT web3_deposits_to_address_check
        CHECK (to_address ~ '^0x[0-9a-f]{40}$'),
    CONSTRAINT web3_deposits_raw_amount_check
        CHECK (raw_amount > 0),
    CONSTRAINT web3_deposits_token_decimals_check
        CHECK (token_decimals >= 0 AND token_decimals <= 255),
    CONSTRAINT web3_deposits_token_amount_check
        CHECK (token_amount > 0),
    CONSTRAINT web3_deposits_credited_amount_check
        CHECK (credited_amount IS NULL OR credited_amount > 0),
    CONSTRAINT web3_deposits_retry_count_check
        CHECK (retry_count >= 0),
    CONSTRAINT web3_deposits_status_check
        CHECK (status IN (
            'detected',
            'confirming',
            'ready_to_credit',
            'crediting',
            'credited',
            'below_minimum',
            'manual_review',
            'orphaned',
            'failed',
            'ignored'
        )),
    CONSTRAINT web3_deposits_event_uniq
        UNIQUE (chain_id, tx_hash, log_index)
);

CREATE INDEX IF NOT EXISTS idx_web3_deposits_worker_claim
    ON web3_deposits (status, next_retry_at, id);

CREATE INDEX IF NOT EXISTS idx_web3_deposits_user_history
    ON web3_deposits (user_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_web3_deposits_block
    ON web3_deposits (block_number, id);

CREATE INDEX IF NOT EXISTS idx_web3_deposits_address_history
    ON web3_deposits (deposit_address_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_web3_deposits_tx_hash_lower
    ON web3_deposits (LOWER(tx_hash));
