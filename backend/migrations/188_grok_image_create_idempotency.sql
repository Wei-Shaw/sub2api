-- Crash-safe synchronous Grok image create intent and response replay.
-- This table is deliberately independent of grok_video_request_owners: image
-- generation/edit responses do not create asynchronous video owner records.
CREATE TABLE IF NOT EXISTS grok_image_create_idempotency (
    user_id BIGINT NOT NULL CHECK (user_id > 0),
    api_key_id BIGINT NOT NULL CHECK (api_key_id > 0),
    group_id BIGINT NOT NULL DEFAULT 0 CHECK (group_id >= 0),
    endpoint VARCHAR(64) NOT NULL
        CHECK (endpoint IN ('images_generations', 'images_edits')),
    idempotency_key_hash CHAR(64) NOT NULL,
    request_hash CHAR(64) NOT NULL,
    upstream_idempotency_key VARCHAR(96) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'processing'
        CHECK (status IN ('processing', 'completed')),
    account_id BIGINT CHECK (account_id > 0),
    response_status SMALLINT,
    response_content_type VARCHAR(255),
    response_body BYTEA,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, api_key_id, group_id, endpoint, idempotency_key_hash),
    CHECK (
        (status = 'processing' AND response_status IS NULL AND response_body IS NULL)
        OR
        (status = 'completed' AND account_id IS NOT NULL
            AND response_status BETWEEN 200 AND 299 AND response_body IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_grok_image_create_idempotency_expiry
    ON grok_image_create_idempotency (expires_at);
CREATE INDEX IF NOT EXISTS idx_grok_image_create_idempotency_account_processing
    ON grok_image_create_idempotency (account_id, updated_at)
    WHERE status = 'processing' AND account_id IS NOT NULL;

COMMENT ON TABLE grok_image_create_idempotency IS
    'Crash-safe caller-scope Grok image create intent, account binding, upstream key, and response replay; expired rows are eligible for bounded concurrent cleanup; never a video owner';
