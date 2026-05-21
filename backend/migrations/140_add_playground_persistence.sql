-- Migration: 140_add_playground_persistence
-- Stores per-user Playground chat sessions/messages and image task history.

CREATE TABLE IF NOT EXISTS playground_chat_sessions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(200) NOT NULL DEFAULT '新会话',
    model VARCHAR(100) NOT NULL DEFAULT '',
    api_key_id BIGINT NULL REFERENCES api_keys(id) ON DELETE SET NULL,
    system_prompt TEXT NOT NULL DEFAULT '',
    use_context BOOLEAN NOT NULL DEFAULT TRUE,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_message_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_playground_chat_sessions_user_last_message
    ON playground_chat_sessions(user_id, last_message_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_playground_chat_sessions_user_updated
    ON playground_chat_sessions(user_id, updated_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_playground_chat_sessions_api_key_id
    ON playground_chat_sessions(api_key_id);
CREATE INDEX IF NOT EXISTS idx_playground_chat_sessions_deleted_at
    ON playground_chat_sessions(deleted_at);

CREATE TABLE IF NOT EXISTS playground_chat_messages (
    id BIGSERIAL PRIMARY KEY,
    session_id BIGINT NOT NULL REFERENCES playground_chat_sessions(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    api_key_id BIGINT NULL REFERENCES api_keys(id) ON DELETE SET NULL,
    role VARCHAR(20) NOT NULL,
    model VARCHAR(100) NOT NULL DEFAULT '',
    content TEXT NOT NULL DEFAULT '',
    content_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    images JSONB NOT NULL DEFAULT '[]'::jsonb,
    usage JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(20) NOT NULL DEFAULT 'success',
    error TEXT NOT NULL DEFAULT '',
    duration_ms INTEGER NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_playground_chat_messages_session_created
    ON playground_chat_messages(session_id, created_at ASC);
CREATE INDEX IF NOT EXISTS idx_playground_chat_messages_user_created
    ON playground_chat_messages(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_playground_chat_messages_api_key_id
    ON playground_chat_messages(api_key_id);
CREATE INDEX IF NOT EXISTS idx_playground_chat_messages_role
    ON playground_chat_messages(role);
CREATE INDEX IF NOT EXISTS idx_playground_chat_messages_status
    ON playground_chat_messages(status);

CREATE TABLE IF NOT EXISTS playground_image_tasks (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    api_key_id BIGINT NULL REFERENCES api_keys(id) ON DELETE SET NULL,
    model VARCHAR(100) NOT NULL DEFAULT '',
    prompt TEXT NOT NULL DEFAULT '',
    quality VARCHAR(20) NOT NULL DEFAULT '',
    size VARCHAR(50) NOT NULL DEFAULT '',
    n INTEGER NOT NULL DEFAULT 1,
    endpoint VARCHAR(100) NOT NULL DEFAULT '/v1/images/generations',
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    request JSONB NOT NULL DEFAULT '{}'::jsonb,
    reference_images JSONB NOT NULL DEFAULT '[]'::jsonb,
    result_images JSONB NOT NULL DEFAULT '[]'::jsonb,
    response JSONB NOT NULL DEFAULT '{}'::jsonb,
    error TEXT NOT NULL DEFAULT '',
    usage JSONB NOT NULL DEFAULT '{}'::jsonb,
    cost DECIMAL(20,10) NOT NULL DEFAULT 0,
    duration_ms INTEGER NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_playground_image_tasks_user_created
    ON playground_image_tasks(user_id, created_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_playground_image_tasks_api_key_id
    ON playground_image_tasks(api_key_id);
CREATE INDEX IF NOT EXISTS idx_playground_image_tasks_status
    ON playground_image_tasks(status);
CREATE INDEX IF NOT EXISTS idx_playground_image_tasks_model
    ON playground_image_tasks(model);
CREATE INDEX IF NOT EXISTS idx_playground_image_tasks_deleted_at
    ON playground_image_tasks(deleted_at);
