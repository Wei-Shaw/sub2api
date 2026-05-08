-- Temporary asynchronous image generation task records.

CREATE TABLE IF NOT EXISTS image_tasks (
    task_id       VARCHAR(40) PRIMARY KEY,
    user_id       BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    api_key_id    BIGINT NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    status        VARCHAR(20) NOT NULL,
    endpoint      VARCHAR(64) NOT NULL,
    model         VARCHAR(128) NOT NULL DEFAULT '',
    prompt        TEXT NOT NULL DEFAULT '',
    file_path     TEXT NOT NULL DEFAULT '',
    mime_type     VARCHAR(64) NOT NULL DEFAULT '',
    byte_size     BIGINT NOT NULL DEFAULT 0,
    error_message TEXT NOT NULL DEFAULT '',
    expires_at    TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_image_tasks_user_created_at
    ON image_tasks(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_image_tasks_expires_at
    ON image_tasks(expires_at);

CREATE INDEX IF NOT EXISTS idx_image_tasks_status
    ON image_tasks(status);
