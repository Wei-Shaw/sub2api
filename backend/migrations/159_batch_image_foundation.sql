CREATE TABLE IF NOT EXISTS batch_image_jobs (
    id BIGSERIAL PRIMARY KEY,
    batch_id VARCHAR(64) NOT NULL UNIQUE,
    user_id BIGINT NOT NULL,
    api_key_id BIGINT,
    account_id BIGINT,
    provider VARCHAR(32) NOT NULL,
    model VARCHAR(128) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'created',
    provider_job_name VARCHAR(512),
    gcs_input_uri VARCHAR(1024),
    gcs_output_uri VARCHAR(1024),
    item_count INTEGER NOT NULL,
    success_count INTEGER NOT NULL DEFAULT 0,
    fail_count INTEGER NOT NULL DEFAULT 0,
    cancelled_count INTEGER NOT NULL DEFAULT 0,
    estimated_cost DECIMAL(20,10) NOT NULL DEFAULT 0,
    hold_amount DECIMAL(20,10),
    actual_cost DECIMAL(20,10),
    currency VARCHAR(16) NOT NULL DEFAULT 'USD',
    hold_id VARCHAR(128),
    idempotency_key VARCHAR(255),
    request_hash VARCHAR(128),
    manifest_hash VARCHAR(128),
    retry_count INTEGER NOT NULL DEFAULT 0,
    version INTEGER NOT NULL DEFAULT 0,
    output_expires_at TIMESTAMPTZ,
    input_deleted_at TIMESTAMPTZ,
    output_deleted_at TIMESTAMPTZ,
    last_error_code VARCHAR(128),
    last_error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    submitted_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    settled_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS batch_image_jobs_user_created_at_idx ON batch_image_jobs (user_id, created_at);
CREATE INDEX IF NOT EXISTS batch_image_jobs_status_idx ON batch_image_jobs (status);
CREATE INDEX IF NOT EXISTS batch_image_jobs_provider_status_idx ON batch_image_jobs (provider, status);
CREATE INDEX IF NOT EXISTS batch_image_jobs_idempotency_key_idx ON batch_image_jobs (idempotency_key)
    WHERE idempotency_key IS NOT NULL AND idempotency_key <> '';
CREATE UNIQUE INDEX IF NOT EXISTS batch_image_jobs_manifest_hash_uq ON batch_image_jobs (manifest_hash)
    WHERE manifest_hash IS NOT NULL AND manifest_hash <> '';
CREATE INDEX IF NOT EXISTS batch_image_jobs_output_expires_at_idx ON batch_image_jobs (output_expires_at);

CREATE TABLE IF NOT EXISTS batch_image_items (
    id BIGSERIAL PRIMARY KEY,
    job_id VARCHAR(64) NOT NULL REFERENCES batch_image_jobs(batch_id) ON DELETE CASCADE,
    custom_id VARCHAR(255) NOT NULL,
    status VARCHAR(32) NOT NULL,
    request_hash VARCHAR(128),
    prompt_preview TEXT,
    provider_source_object VARCHAR(1024),
    source_line_number INTEGER,
    source_byte_offset BIGINT,
    source_byte_length BIGINT,
    mime_type VARCHAR(128),
    file_extension VARCHAR(32),
    image_count INTEGER NOT NULL DEFAULT 0,
    error_code VARCHAR(128),
    error_message TEXT,
    billed_amount DECIMAL(20,10),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    indexed_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS batch_image_items_job_custom_uq ON batch_image_items (job_id, custom_id);
CREATE INDEX IF NOT EXISTS batch_image_items_job_status_idx ON batch_image_items (job_id, status);
CREATE INDEX IF NOT EXISTS batch_image_items_provider_source_object_idx ON batch_image_items (provider_source_object);

CREATE TABLE IF NOT EXISTS batch_image_events (
    id BIGSERIAL PRIMARY KEY,
    job_id VARCHAR(64) NOT NULL REFERENCES batch_image_jobs(batch_id) ON DELETE CASCADE,
    event_type VARCHAR(64) NOT NULL,
    payload JSONB,
    event_hash VARCHAR(128),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS batch_image_events_job_created_at_idx ON batch_image_events (job_id, created_at);
CREATE INDEX IF NOT EXISTS batch_image_events_event_type_idx ON batch_image_events (event_type);
CREATE UNIQUE INDEX IF NOT EXISTS batch_image_events_job_event_hash_uq ON batch_image_events (job_id, event_hash)
    WHERE event_hash IS NOT NULL AND event_hash <> '';
