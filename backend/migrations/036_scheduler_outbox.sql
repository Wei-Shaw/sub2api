CREATE TABLE IF NOT EXISTS scheduler_outbox (
    id BIGSERIAL PRIMARY KEY,
    event_type TEXT NOT NULL,
    account_id BIGINT NULL,
    group_id BIGINT NULL,
    payload JSONB NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_scheduler_outbox_created_at ON scheduler_outbox (created_at);
