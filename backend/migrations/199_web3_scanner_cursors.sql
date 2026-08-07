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
    CONSTRAINT web3_scanner_cursors_chain_id_check
        CHECK (chain_id > 0),
    CONSTRAINT web3_scanner_cursors_token_contract_check
        CHECK (token_contract ~ '^0x[0-9a-f]{40}$'),
    CONSTRAINT web3_scanner_cursors_scan_start_check
        CHECK (scan_start_block >= 0),
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
