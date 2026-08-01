-- Durable usage billing outbox for request-time settlement and restart-safe retries.
-- The usage_logs row and this outbox row are inserted in one transaction.
CREATE TABLE IF NOT EXISTS usage_billing_settlements (
    id BIGSERIAL PRIMARY KEY,
    request_id VARCHAR(128) NOT NULL,
    api_key_id BIGINT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    usage_log_id BIGINT NOT NULL REFERENCES usage_logs(id) ON DELETE CASCADE,
    request_fingerprint VARCHAR(128) NOT NULL DEFAULT '',
    command JSONB NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_until TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    applied_at TIMESTAMPTZ,
    CONSTRAINT usage_billing_settlements_status_check
        CHECK (status IN ('pending', 'processing', 'retry', 'applied')),
    CONSTRAINT usage_billing_settlements_request_key
        UNIQUE (request_id, api_key_id),
    CONSTRAINT usage_billing_settlements_usage_log_key
        UNIQUE (usage_log_id)
);

CREATE INDEX IF NOT EXISTS usage_billing_settlements_due_idx
    ON usage_billing_settlements (status, next_attempt_at, id)
    WHERE status IN ('pending', 'processing', 'retry');

CREATE INDEX IF NOT EXISTS usage_billing_settlements_locked_idx
    ON usage_billing_settlements (locked_until, id)
    WHERE status = 'processing';
