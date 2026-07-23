-- Durable one-shot background task executions.
--
-- dedupe_locked remains true after an external side effect may have happened.
-- Together with the active-status predicate this prevents a second logical
-- task from being created with a different upstream idempotency key.

CREATE TABLE IF NOT EXISTS background_task_runs (
    id BIGSERIAL PRIMARY KEY,
    task_type VARCHAR(64) NOT NULL,
    resource_type VARCHAR(64) NOT NULL,
    resource_id VARCHAR(128) NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    display JSONB NOT NULL DEFAULT '{}'::jsonb,
    run_at TIMESTAMPTZ NOT NULL,
    status VARCHAR(24) NOT NULL DEFAULT 'pending',
    attempt_count INT NOT NULL DEFAULT 0,
    dispatch_count INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 5,
    dedupe_key VARCHAR(512) NOT NULL,
    idempotency_key VARCHAR(128),
    creation_request_key VARCHAR(128),
    dedupe_locked BOOLEAN NOT NULL DEFAULT false,
    claim_owner VARCHAR(128),
    claim_version BIGINT NOT NULL DEFAULT 0,
    lease_until TIMESTAMPTZ,
    first_dispatch_at TIMESTAMPTZ,
    last_dispatch_at TIMESTAMPTZ,
    result_code VARCHAR(64),
    result JSONB,
    last_error_code VARCHAR(64),
    last_error_message TEXT,
    created_by BIGINT NOT NULL,
    canceled_by BIGINT,
    canceled_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT background_task_runs_status_check CHECK (
        status IN (
            'pending', 'running', 'retry_wait', 'succeeded',
            'skipped', 'failed', 'canceled', 'indeterminate'
        )
    ),
    CONSTRAINT background_task_runs_attempts_check CHECK (
        attempt_count >= 0 AND dispatch_count >= 0 AND max_attempts > 0
    )
);

-- Keep this migration usable in development databases that applied an earlier
-- draft before creation-request idempotency was added.
ALTER TABLE background_task_runs
    ADD COLUMN IF NOT EXISTS creation_request_key VARCHAR(128);

-- One task may be returned for more than one idempotent creation request. For
-- example, a second key can resolve to an existing active dedupe owner. Keep
-- every accepted request key durable so a lost HTTP response cannot later
-- create a different task after that owner reaches a terminal state.
CREATE TABLE IF NOT EXISTS background_task_creation_requests (
    request_key VARCHAR(128) PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES background_task_runs(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_background_task_creation_requests_task
    ON background_task_creation_requests (task_id);

INSERT INTO background_task_creation_requests (request_key, task_id)
SELECT creation_request_key, id
FROM background_task_runs
WHERE creation_request_key IS NOT NULL
ON CONFLICT (request_key) DO NOTHING;

CREATE INDEX IF NOT EXISTS idx_background_task_runs_due
    ON background_task_runs (run_at, id)
    WHERE status IN ('pending', 'retry_wait');

CREATE INDEX IF NOT EXISTS idx_background_task_runs_expired_leases
    ON background_task_runs (lease_until, id)
    WHERE status = 'running';

CREATE INDEX IF NOT EXISTS idx_background_task_runs_resource
    ON background_task_runs (resource_type, resource_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_background_task_runs_list
    ON background_task_runs (created_at DESC, id DESC);

CREATE UNIQUE INDEX IF NOT EXISTS uq_background_task_runs_active_dedupe
    ON background_task_runs (task_type, dedupe_key)
    WHERE dedupe_locked OR status IN ('pending', 'running', 'retry_wait');

CREATE UNIQUE INDEX IF NOT EXISTS uq_background_task_runs_idempotency_key
    ON background_task_runs (idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_background_task_runs_creation_request_key
    ON background_task_runs (creation_request_key)
    WHERE creation_request_key IS NOT NULL;

COMMENT ON TABLE background_task_runs IS
    'Durable finite background task executions with lease fencing and external-effect idempotency.';
