CREATE TABLE IF NOT EXISTS cache_invalidation_outbox (
    id              BIGSERIAL PRIMARY KEY,
    event_type      VARCHAR(64) NOT NULL,
    aggregate_type  VARCHAR(64) NOT NULL,
    aggregate_id    BIGINT,
    reason          VARCHAR(128) NOT NULL,
    cache_types     TEXT[] NOT NULL,
    payload         JSONB NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'pending',
    attempts        INTEGER NOT NULL DEFAULT 0,
    max_attempts    INTEGER NOT NULL DEFAULT 12,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_at       TIMESTAMPTZ,
    locked_by       VARCHAR(128),
    processed_at    TIMESTAMPTZ,
    last_error      TEXT,
    idempotency_key VARCHAR(200),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_cache_invalidation_outbox_status CHECK (
        status IN ('pending', 'processing', 'succeeded', 'failed', 'dead')
    ),
    CONSTRAINT chk_cache_invalidation_outbox_attempts CHECK (
        attempts >= 0
        AND max_attempts > 0
        AND (attempts <= max_attempts OR status IN ('dead', 'succeeded'))
    ),
    CONSTRAINT chk_cache_invalidation_outbox_cache_types CHECK (
        array_length(cache_types, 1) IS NOT NULL
    )
);

CREATE INDEX IF NOT EXISTS idx_cache_invalidation_outbox_ready
    ON cache_invalidation_outbox (status, next_attempt_at, id)
    WHERE status IN ('pending', 'failed');

CREATE INDEX IF NOT EXISTS idx_cache_invalidation_outbox_locked
    ON cache_invalidation_outbox (locked_at)
    WHERE status = 'processing';

CREATE INDEX IF NOT EXISTS idx_cache_invalidation_outbox_created_at
    ON cache_invalidation_outbox (created_at);

CREATE UNIQUE INDEX IF NOT EXISTS idx_cache_invalidation_outbox_idempotency
    ON cache_invalidation_outbox (idempotency_key)
    WHERE idempotency_key IS NOT NULL;
