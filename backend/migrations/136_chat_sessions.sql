-- Add server-backed chat history tables for the user chat workbench.

CREATE TABLE IF NOT EXISTS chat_sessions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    title VARCHAR(160) NOT NULL,
    model VARCHAR(100) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT chat_sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT chat_sessions_api_key_id_fkey FOREIGN KEY (api_key_id) REFERENCES api_keys(id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS chat_messages (
    id BIGSERIAL PRIMARY KEY,
    session_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    role VARCHAR(20) NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'completed',
    model VARCHAR(100) NULL,
    duration_ms INTEGER NULL,
    usage_log_id BIGINT NULL,
    actual_cost DECIMAL(20,10) NULL,
    error_message TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chat_messages_session_id_fkey FOREIGN KEY (session_id) REFERENCES chat_sessions(id) ON DELETE CASCADE,
    CONSTRAINT chat_messages_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT chat_messages_usage_log_id_fkey FOREIGN KEY (usage_log_id) REFERENCES usage_logs(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS chat_sessions_user_updated_at_idx ON chat_sessions (user_id, updated_at);
CREATE INDEX IF NOT EXISTS chat_sessions_user_expires_at_idx ON chat_sessions (user_id, expires_at);
CREATE INDEX IF NOT EXISTS chat_sessions_user_deleted_at_idx ON chat_sessions (user_id, deleted_at);
CREATE INDEX IF NOT EXISTS chat_sessions_api_key_id_idx ON chat_sessions (api_key_id);
CREATE INDEX IF NOT EXISTS chat_sessions_status_idx ON chat_sessions (status);

CREATE INDEX IF NOT EXISTS chat_messages_session_created_at_idx ON chat_messages (session_id, created_at);
CREATE INDEX IF NOT EXISTS chat_messages_user_created_at_idx ON chat_messages (user_id, created_at);
CREATE INDEX IF NOT EXISTS chat_messages_usage_log_id_idx ON chat_messages (usage_log_id);
CREATE INDEX IF NOT EXISTS chat_messages_status_idx ON chat_messages (status);
