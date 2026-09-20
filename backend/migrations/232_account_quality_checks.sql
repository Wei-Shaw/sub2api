CREATE TABLE IF NOT EXISTS account_quality_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    model VARCHAR(200) NOT NULL DEFAULT 'gpt-6-astra',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO account_quality_settings(id) VALUES (1) ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS account_quality_schedule (
    account_id BIGINT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    next_run_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_until TIMESTAMPTZ,
    lease_token VARCHAR(64)
);

CREATE TABLE IF NOT EXISTS account_quality_runs (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    model VARCHAR(200) NOT NULL,
    status VARCHAR(24) NOT NULL DEFAULT 'running',
    cases JSONB NOT NULL DEFAULT '[]'::jsonb,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_account_quality_runs_account ON account_quality_runs(account_id, id DESC);
