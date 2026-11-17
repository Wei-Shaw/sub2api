-- Scheduled station evidence is separate from local billing and usage logs.
CREATE TABLE IF NOT EXISTS fork_upstream_activity_state (
    account_id BIGINT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    identity TEXT NOT NULL DEFAULT '',
    completed_through TIMESTAMPTZ,
    last_attempt TIMESTAMPTZ,
    next_attempt TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    last_error TEXT NOT NULL DEFAULT '',
    wallet_balance TEXT,
    wallet_checked_at TIMESTAMPTZ,
    wallet_error TEXT NOT NULL DEFAULT '',
    lease_token TEXT,
    lease_until TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS fork_upstream_activity_records (
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    identity TEXT NOT NULL,
    station_id TEXT NOT NULL,
    request_id TEXT NOT NULL DEFAULT '',
    record JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (account_id, identity, station_id)
);
CREATE INDEX IF NOT EXISTS idx_fork_activity_request
    ON fork_upstream_activity_records(account_id, identity, request_id);
