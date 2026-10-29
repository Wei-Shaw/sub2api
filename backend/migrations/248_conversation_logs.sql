-- Conversation content is deliberately separate from billing/usage logs.
-- No data is imported from an existing deployment. Capture is opt-in.
CREATE TABLE IF NOT EXISTS conversation_log_config (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    retention_days INTEGER NOT NULL DEFAULT 7 CHECK (retention_days BETWEEN 1 AND 365),
    max_body_bytes INTEGER NOT NULL DEFAULT 1048576 CHECK (max_body_bytes BETWEEN 16384 AND 2097152)
);
INSERT INTO conversation_log_config (id) VALUES (1) ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS conversation_logs (
    id BIGSERIAL PRIMARY KEY,
    request_id VARCHAR(128) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    user_label VARCHAR(320) NOT NULL DEFAULT '',
    key_name VARCHAR(256) NOT NULL DEFAULT '',
    endpoint VARCHAR(256) NOT NULL,
    model VARCHAR(256) NOT NULL DEFAULT '',
    stream BOOLEAN NOT NULL,
    status_code INTEGER NOT NULL,
    outcome VARCHAR(32) NOT NULL,
    duration_ms BIGINT NOT NULL,
    prompt TEXT NOT NULL DEFAULT '',
    answer TEXT NOT NULL DEFAULT '',
    request_body TEXT NOT NULL DEFAULT '',
    response_body TEXT NOT NULL DEFAULT '',
    tool_calls JSONB NOT NULL DEFAULT '[]',
    request_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    response_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    request_bytes BIGINT NOT NULL,
    response_bytes BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS conversation_logs_created_idx ON conversation_logs(created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS conversation_logs_user_idx ON conversation_logs(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS conversation_logs_request_idx ON conversation_logs(request_id);
