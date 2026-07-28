-- Durable owner binding for asynchronous Grok video tasks.
-- Redis is only an acceleration cache; status/content authorization and routing
-- always resolve this table by the complete caller scope.

CREATE TABLE IF NOT EXISTS grok_video_request_owners (
    request_id VARCHAR(255) NOT NULL,
    user_id BIGINT NOT NULL CHECK (user_id > 0),
    api_key_id BIGINT NOT NULL CHECK (api_key_id > 0),
    group_id BIGINT NOT NULL DEFAULT 0 CHECK (group_id >= 0),
    account_id BIGINT NOT NULL CHECK (account_id > 0),
    expires_at TIMESTAMPTZ NOT NULL,
    last_accessed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    terminal_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (request_id, user_id, api_key_id, group_id)
);

CREATE INDEX IF NOT EXISTS idx_grok_video_request_owners_expiry
    ON grok_video_request_owners (expires_at);
CREATE INDEX IF NOT EXISTS idx_grok_video_request_owners_account_active
    ON grok_video_request_owners (account_id, expires_at);
CREATE INDEX IF NOT EXISTS idx_grok_video_request_owners_terminal_cleanup
    ON grok_video_request_owners (expires_at, terminal_at)
    WHERE terminal_at IS NOT NULL;

COMMENT ON TABLE grok_video_request_owners IS
    'Authoritative immutable request owner for Grok video status/content routing; successful access renews a 7-day recovery window and terminal rows retain 7 days before bounded cleanup';

-- A create intent is committed before the upstream request is sent. The
-- derived upstream idempotency key is stable across process restarts, so an
-- ambiguous upstream acceptance can be replayed against the same account
-- without creating a second paid task. Raw caller idempotency keys are never
-- stored.
CREATE TABLE IF NOT EXISTS grok_video_create_idempotency (
    user_id BIGINT NOT NULL CHECK (user_id > 0),
    api_key_id BIGINT NOT NULL CHECK (api_key_id > 0),
    group_id BIGINT NOT NULL DEFAULT 0 CHECK (group_id >= 0),
    endpoint VARCHAR(64) NOT NULL,
    idempotency_key_hash CHAR(64) NOT NULL,
    request_hash CHAR(64) NOT NULL,
    upstream_idempotency_key VARCHAR(96) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'processing'
        CHECK (status IN ('processing', 'completed')),
    account_id BIGINT CHECK (account_id > 0),
    request_id VARCHAR(255),
    response_status SMALLINT,
    response_content_type VARCHAR(255),
    response_body BYTEA,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, api_key_id, group_id, endpoint, idempotency_key_hash),
    CHECK (
        (status = 'processing' AND request_id IS NULL AND response_status IS NULL AND response_body IS NULL)
        OR
        (status = 'completed' AND account_id IS NOT NULL AND request_id IS NOT NULL AND response_status BETWEEN 200 AND 299 AND response_body IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_grok_video_create_idempotency_expiry
    ON grok_video_create_idempotency (expires_at);
CREATE INDEX IF NOT EXISTS idx_grok_video_create_idempotency_account_processing
    ON grok_video_create_idempotency (account_id, updated_at)
    WHERE status = 'processing' AND account_id IS NOT NULL;

COMMENT ON TABLE grok_video_create_idempotency IS
    'Crash-safe caller-scope video create intent and replay response; raw Idempotency-Key values are hashed; expired rows are eligible for bounded concurrent cleanup';
