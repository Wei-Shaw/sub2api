-- Official upstream costs are additive bookkeeping. Never rewrite usage_logs,
-- user balances or subscription consumption from an aggregate provider invoice.
CREATE TABLE IF NOT EXISTS upstream_billing_connections (
    id BIGSERIAL PRIMARY KEY,
    owner_id BIGINT NOT NULL,
    name VARCHAR(120) NOT NULL,
    provider VARCHAR(32) NOT NULL,
    settings JSONB NOT NULL DEFAULT '{}',
    encrypted_secrets TEXT NOT NULL,
    scope_key VARCHAR(64) NOT NULL UNIQUE,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    sync_interval_hours INTEGER NOT NULL DEFAULT 24 CHECK (sync_interval_hours BETWEEN 1 AND 168),
    last_synced_at TIMESTAMPTZ,
    next_sync_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error VARCHAR(500) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS upstream_billing_bindings (
    connection_id BIGINT NOT NULL REFERENCES upstream_billing_connections(id) ON DELETE CASCADE,
    resource_id VARCHAR(2048) NOT NULL,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    PRIMARY KEY (connection_id, account_id),
    -- One upstream API-key account cannot be charged by two integrations.
    UNIQUE (account_id)
);

-- Successful revisions are immutable. The month pointer switches atomically
-- only after the entire provider fetch and allocation have succeeded.
CREATE TABLE IF NOT EXISTS upstream_billing_runs (
    id BIGSERIAL PRIMARY KEY,
    connection_id BIGINT NOT NULL REFERENCES upstream_billing_connections(id),
    month VARCHAR(7) NOT NULL,
    fingerprint VARCHAR(64) NOT NULL,
    bindings_snapshot JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (connection_id, month, id)
);

CREATE TABLE IF NOT EXISTS upstream_billing_months (
    connection_id BIGINT NOT NULL REFERENCES upstream_billing_connections(id),
    month VARCHAR(7) NOT NULL,
    active_run_id BIGINT NOT NULL,
    PRIMARY KEY (connection_id, month),
    FOREIGN KEY (connection_id, month, active_run_id)
        REFERENCES upstream_billing_runs(connection_id, month, id)
);

CREATE TABLE IF NOT EXISTS upstream_billing_bills (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL REFERENCES upstream_billing_runs(id),
    provider_bill_id VARCHAR(256) NOT NULL,
    resource_id VARCHAR(2048) NOT NULL,
    description VARCHAR(2048) NOT NULL,
    source_amount VARCHAR(80) NOT NULL,
    raw_source JSONB NOT NULL DEFAULT '{}',
    usage_evidence JSONB NOT NULL DEFAULT '{}',
    amount NUMERIC(38,8) NOT NULL,
    currency VARCHAR(3) NOT NULL,
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL CHECK (period_end > period_start),
    UNIQUE (run_id, provider_bill_id)
);

-- Deliberately snapshot account/key labels and identifiers without foreign
-- keys: deleting keys or pruning usage logs must not erase past reconciliation.
CREATE TABLE IF NOT EXISTS upstream_billing_allocations (
    id BIGSERIAL PRIMARY KEY,
    bill_id BIGINT NOT NULL REFERENCES upstream_billing_bills(id),
    account_id BIGINT NOT NULL DEFAULT 0,
    account_name VARCHAR(255) NOT NULL DEFAULT '',
    api_key_id BIGINT NOT NULL DEFAULT 0,
    api_key_name VARCHAR(255) NOT NULL DEFAULT '',
    user_id BIGINT NOT NULL DEFAULT 0,
    requests BIGINT NOT NULL DEFAULT 0,
    tokens BIGINT NOT NULL DEFAULT 0,
    local_cost NUMERIC(38,10) NOT NULL DEFAULT 0,
    allocated_cost NUMERIC(38,8) NOT NULL DEFAULT 0,
    official_tokens BIGINT,
    local_matched_tokens BIGINT,
    token_status VARCHAR(64) NOT NULL DEFAULT 'unsupported',
    method VARCHAR(32) NOT NULL CHECK (method IN ('local_cost_weighted', 'token_weighted', 'official_token_weighted', 'unmatched')),
    UNIQUE (bill_id, account_id, api_key_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_upstream_billing_due
    ON upstream_billing_connections(next_sync_at) WHERE enabled;
CREATE INDEX IF NOT EXISTS idx_upstream_billing_runs_month
    ON upstream_billing_runs(connection_id, month);
CREATE INDEX IF NOT EXISTS idx_upstream_billing_allocations_user
    ON upstream_billing_allocations(user_id, bill_id);
CREATE INDEX IF NOT EXISTS idx_upstream_billing_allocations_bill
    ON upstream_billing_allocations(bill_id);
