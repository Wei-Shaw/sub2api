-- New OpenAI video tasks only. No submission/idempotency-key records here.
-- Deliberately no cascading foreign keys: deleting a key/account must never
-- silently discard an unsettled task. Credentials are loaded by ID at runtime.
CREATE TABLE IF NOT EXISTS video_tasks (
    task_id VARCHAR(512) NOT NULL,
    user_id BIGINT NOT NULL CHECK (user_id > 0),
    api_key_id BIGINT NOT NULL CHECK (api_key_id > 0),
    group_id BIGINT,
    account_id BIGINT NOT NULL CHECK (account_id > 0),
    subscription_id BIGINT,
    routing_fingerprint CHAR(64) NOT NULL,
    pending JSONB NOT NULL CHECK (jsonb_typeof(pending) = 'object'),
    cost_snapshot JSONB NOT NULL CHECK (jsonb_typeof(cost_snapshot) = 'object'),
    completion JSONB,
    state VARCHAR(16) NOT NULL DEFAULT 'pending'
        CHECK (state IN ('pending', 'ready', 'completed', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_token TEXT,
    lease_until TIMESTAMPTZ,
    last_error VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    PRIMARY KEY (task_id, user_id, api_key_id),
    CHECK (state NOT IN ('ready', 'completed') OR completion IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS idx_video_tasks_due
    ON video_tasks (next_attempt_at, lease_until)
    WHERE state IN ('pending', 'ready');
