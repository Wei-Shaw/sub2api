-- Durable usage billing queue. PostgreSQL WAL is the source of truth; Redis is
-- only an optional low-latency pending-usage overlay.

CREATE TABLE IF NOT EXISTS usage_billing_jobs (
    id BIGSERIAL PRIMARY KEY,
    request_id VARCHAR(255) NOT NULL,
    api_key_id BIGINT NOT NULL,
    request_fingerprint VARCHAR(64) NOT NULL,
    payload JSONB NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_usage_billing_jobs_request_api_key UNIQUE (request_id, api_key_id)
);

CREATE INDEX IF NOT EXISTS idx_usage_billing_jobs_ready
    ON usage_billing_jobs (available_at, id);

CREATE TABLE IF NOT EXISTS usage_billing_dead_letters (
    id BIGSERIAL PRIMARY KEY,
    source_job_id BIGINT NOT NULL,
    request_id VARCHAR(255) NOT NULL,
    api_key_id BIGINT NOT NULL,
    request_fingerprint VARCHAR(64) NOT NULL,
    payload JSONB NOT NULL,
    attempts INTEGER NOT NULL,
    reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    failed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_usage_billing_dead_letters_request_api_key UNIQUE (request_id, api_key_id)
);

CREATE INDEX IF NOT EXISTS idx_usage_billing_dead_letters_failed_at
    ON usage_billing_dead_letters (failed_at);
