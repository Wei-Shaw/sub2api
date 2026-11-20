-- Local OpenAI-compatible files and durable, at-most-once batch execution.
-- Files are stored in PostgreSQL so every application replica sees the same data.
CREATE TABLE IF NOT EXISTS openai_files (
    id TEXT PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    purpose TEXT NOT NULL,
    filename TEXT NOT NULL,
    content BYTEA NOT NULL,
    created_at BIGINT NOT NULL,
    expires_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS openai_files_user_created_idx ON openai_files(user_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS openai_files_expiry_idx ON openai_files(expires_at);

CREATE TABLE IF NOT EXISTS openai_batches (
    id TEXT PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    api_key_id BIGINT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    remote_ip TEXT NOT NULL,
    status TEXT NOT NULL,
    payload JSONB NOT NULL,
    created_at BIGINT NOT NULL,
    output_ttl_seconds BIGINT NOT NULL DEFAULT 2592000,
    result_bytes BIGINT NOT NULL DEFAULT 0,
    lease_token TEXT,
    lease_until TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS openai_batches_user_created_idx ON openai_batches(user_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS openai_batches_queue_idx ON openai_batches(status, lease_until, created_at);

CREATE TABLE IF NOT EXISTS openai_batch_items (
    batch_id TEXT NOT NULL REFERENCES openai_batches(id) ON DELETE CASCADE,
    ordinal INTEGER NOT NULL,
    custom_id TEXT NOT NULL,
    body JSONB NOT NULL,
    state TEXT NOT NULL DEFAULT 'pending',
    result JSON,
    succeeded BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY(batch_id, ordinal),
    UNIQUE(batch_id, custom_id)
);
