-- Web3 Deposit: persistent, recoverable bounded rescan jobs.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

CREATE TABLE IF NOT EXISTS web3_rescan_jobs (
    id               BIGSERIAL PRIMARY KEY,
    network_key      VARCHAR(64) NOT NULL,
    asset_key        VARCHAR(64) NOT NULL,
    from_block       BIGINT NOT NULL,
    to_block         BIGINT NOT NULL,
    status           VARCHAR(16) NOT NULL DEFAULT 'pending',
    requested_by     BIGINT NOT NULL DEFAULT 0,
    attempt_count    INTEGER NOT NULL DEFAULT 0,
    event_count      INTEGER NOT NULL DEFAULT 0,
    matched_count    INTEGER NOT NULL DEFAULT 0,
    deposit_count    INTEGER NOT NULL DEFAULT 0,
    error_message    VARCHAR(2000),
    lease_expires_at TIMESTAMPTZ,
    started_at       TIMESTAMPTZ,
    completed_at     TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT web3_rescan_jobs_range_check
        CHECK (from_block >= 0 AND to_block >= from_block),
    CONSTRAINT web3_rescan_jobs_status_check
        CHECK (status IN ('pending', 'running', 'succeeded', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_web3_rescan_jobs_claim
    ON web3_rescan_jobs (status, lease_expires_at, id);

CREATE INDEX IF NOT EXISTS idx_web3_rescan_jobs_created
    ON web3_rescan_jobs (created_at DESC, id DESC);
