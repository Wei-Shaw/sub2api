-- Web3 Deposit: address allocation, scanning, finalization, accounting,
-- operational alerting, and recoverable bounded rescans.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

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
        CHECK (next_derivation_index >= 0 AND next_derivation_index <= 2147483648),
    CONSTRAINT web3_deposit_wallets_status_check
        CHECK (status IN ('active', 'disabled'))
);

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
    token_amount NUMERIC(78,6) NOT NULL,
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
    CONSTRAINT web3_deposits_chain_id_check CHECK (chain_id > 0),
    CONSTRAINT web3_deposits_token_contract_check
        CHECK (token_contract ~ '^0x[0-9a-f]{40}$'),
    CONSTRAINT web3_deposits_tx_hash_check
        CHECK (tx_hash ~ '^0x[0-9a-f]{64}$'),
    CONSTRAINT web3_deposits_log_index_check CHECK (log_index >= 0),
    CONSTRAINT web3_deposits_block_number_check CHECK (block_number >= 0),
    CONSTRAINT web3_deposits_block_hash_check
        CHECK (block_hash ~ '^0x[0-9a-f]{64}$'),
    CONSTRAINT web3_deposits_from_address_check
        CHECK (from_address ~ '^0x[0-9a-f]{40}$'),
    CONSTRAINT web3_deposits_to_address_check
        CHECK (to_address ~ '^0x[0-9a-f]{40}$'),
    CONSTRAINT web3_deposits_raw_amount_check CHECK (raw_amount > 0),
    CONSTRAINT web3_deposits_token_decimals_check
        CHECK (token_decimals >= 0 AND token_decimals <= 255),
    CONSTRAINT web3_deposits_token_amount_check CHECK (token_amount > 0),
    CONSTRAINT web3_deposits_credited_amount_check
        CHECK (credited_amount IS NULL OR credited_amount > 0),
    CONSTRAINT web3_deposits_retry_count_check CHECK (retry_count >= 0),
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

CREATE TABLE IF NOT EXISTS web3_scanner_cursors (
    id BIGSERIAL PRIMARY KEY,
    scanner_key VARCHAR(128) NOT NULL UNIQUE,
    chain_id BIGINT NOT NULL,
    token_contract VARCHAR(42) NOT NULL,
    scan_start_block BIGINT NOT NULL,
    last_scanned_block BIGINT NOT NULL,
    last_finalized_block BIGINT NOT NULL,
    lease_owner VARCHAR(128),
    lease_token VARCHAR(128),
    lease_expires_at TIMESTAMPTZ,
    last_error TEXT,
    last_success_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT web3_scanner_cursors_chain_asset_uniq
        UNIQUE (chain_id, token_contract),
    CONSTRAINT web3_scanner_cursors_chain_id_check CHECK (chain_id > 0),
    CONSTRAINT web3_scanner_cursors_token_contract_check
        CHECK (token_contract ~ '^0x[0-9a-f]{40}$'),
    CONSTRAINT web3_scanner_cursors_scan_start_check CHECK (scan_start_block >= 0),
    CONSTRAINT web3_scanner_cursors_scanned_monotonic_check
        CHECK (last_scanned_block >= scan_start_block),
    CONSTRAINT web3_scanner_cursors_finalized_monotonic_check
        CHECK (
            last_finalized_block >= scan_start_block
            AND last_finalized_block <= last_scanned_block
        ),
    CONSTRAINT web3_scanner_cursors_lease_consistency_check
        CHECK (
            (
                lease_owner IS NULL
                AND lease_token IS NULL
                AND lease_expires_at IS NULL
            )
            OR
            (
                lease_owner IS NOT NULL
                AND lease_token IS NOT NULL
                AND lease_expires_at IS NOT NULL
            )
        )
);

CREATE INDEX IF NOT EXISTS idx_web3_scanner_cursors_lease_expiry
    ON web3_scanner_cursors (lease_expires_at);

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
    CONSTRAINT web3_user_balances_id_user_uniq UNIQUE (id, user_id),
    CONSTRAINT web3_user_balances_user_asset_uniq UNIQUE (user_id, asset_key),
    CONSTRAINT web3_user_balances_asset_key_check
        CHECK (asset_key ~ '^[a-z0-9_]{1,64}$'),
    CONSTRAINT web3_user_balances_available_amount_check
        CHECK (available_amount >= 0),
    CONSTRAINT web3_user_balances_total_deposited_check
        CHECK (total_deposited >= 0),
    CONSTRAINT web3_user_balances_total_transferred_check
        CHECK (total_transferred >= 0),
    CONSTRAINT web3_user_balances_version_check CHECK (balance_version >= 0),
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
    CONSTRAINT web3_balance_transfers_amount_check CHECK (amount > 0),
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

CREATE TABLE IF NOT EXISTS web3_rescan_jobs (
    id BIGSERIAL PRIMARY KEY,
    network_key VARCHAR(64) NOT NULL,
    asset_key VARCHAR(64) NOT NULL,
    from_block BIGINT NOT NULL,
    to_block BIGINT NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    requested_by BIGINT NOT NULL DEFAULT 0,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    event_count INTEGER NOT NULL DEFAULT 0,
    matched_count INTEGER NOT NULL DEFAULT 0,
    deposit_count INTEGER NOT NULL DEFAULT 0,
    error_message VARCHAR(2000),
    lease_expires_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT web3_rescan_jobs_range_check
        CHECK (from_block >= 0 AND to_block >= from_block),
    CONSTRAINT web3_rescan_jobs_status_check
        CHECK (status IN ('pending', 'running', 'succeeded', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_web3_rescan_jobs_claim
    ON web3_rescan_jobs (status, lease_expires_at, id);
CREATE INDEX IF NOT EXISTS idx_web3_rescan_jobs_created
    ON web3_rescan_jobs (created_at DESC, id DESC);

INSERT INTO ops_alert_rules (
    name, description, enabled, metric_type, operator, threshold,
    window_minutes, sustained_minutes, severity, notify_email, cooldown_minutes,
    created_at, updated_at
) VALUES
    (
        'Web3 RPC无健康端点',
        '当 Web3 充值 RPC endpoint 池无健康端点且持续 2 分钟时触发告警',
        true, 'web3_rpc_unhealthy', '>=', 1.0, 1, 2, 'P0', true, 10, NOW(), NOW()
    ),
    (
        'Web3 Scanner区块延迟过高',
        '当 Web3 充值 scanner 延迟超过 120 个区块且持续 5 分钟时触发告警',
        true, 'web3_scanner_lag_blocks', '>', 120.0, 1, 5, 'P1', true, 20, NOW(), NOW()
    ),
    (
        'Web3 Finalizer区块延迟过高',
        '当 Web3 充值 finalizer 延迟超过 60 个 finalized 区块且持续 5 分钟时触发告警',
        true, 'web3_finalizer_lag_blocks', '>', 60.0, 1, 5, 'P1', true, 20, NOW(), NOW()
    ),
    (
        'Web3人工审核积压',
        '当 Web3 充值 manual_review 记录超过 10 条且持续 10 分钟时触发告警',
        true, 'web3_manual_review_count', '>', 10.0, 1, 10, 'P2', true, 30, NOW(), NOW()
    ),
    (
        'Web3入账失败',
        '当 Web3 充值 credit failed 计数大于 0 且持续 1 分钟时触发告警',
        true, 'web3_credit_failures_total', '>', 0.0, 1, 1, 'P1', true, 20, NOW(), NOW()
    )
ON CONFLICT (name) DO NOTHING;
