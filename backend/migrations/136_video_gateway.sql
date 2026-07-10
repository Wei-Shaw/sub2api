-- Video gateway P0 tables. The video surface is a thin adapter around the
-- existing user/auth/logging patterns while keeping the LLM scheduler untouched.

CREATE TABLE IF NOT EXISTS video_provider_accounts (
    id BIGSERIAL PRIMARY KEY,
    provider TEXT NOT NULL CHECK (provider IN ('mock', 'seedance', 'kling')),
    display_name TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    encrypted_api_key TEXT NOT NULL DEFAULT '',
    masked_key TEXT NOT NULL DEFAULT '',
    base_url TEXT NOT NULL DEFAULT '',
    default_model TEXT NOT NULL DEFAULT '',
    rate_limit_per_minute INTEGER NOT NULL DEFAULT 60 CHECK (rate_limit_per_minute >= 0),
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_video_provider_accounts_provider_name
    ON video_provider_accounts (provider, display_name);
CREATE INDEX IF NOT EXISTS idx_video_provider_accounts_provider_enabled
    ON video_provider_accounts (provider, enabled);

CREATE TABLE IF NOT EXISTS video_tasks (
    id BIGSERIAL PRIMARY KEY,
    provider_account_id BIGINT NOT NULL REFERENCES video_provider_accounts(id),
    provider TEXT NOT NULL CHECK (provider IN ('mock', 'seedance', 'kling')),
    model TEXT NOT NULL,
    task_type TEXT NOT NULL CHECK (task_type IN ('text_to_video', 'image_to_video', 'reference_to_video')),
    prompt TEXT NOT NULL,
    negative_prompt TEXT NOT NULL DEFAULT '',
    reference_image_url TEXT NOT NULL DEFAULT '',
    reference_video_url TEXT NOT NULL DEFAULT '',
    aspect_ratio TEXT NOT NULL DEFAULT '16:9',
    duration INTEGER NOT NULL DEFAULT 5 CHECK (duration >= 0),
    resolution TEXT NOT NULL DEFAULT '720p',
    status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'submitted', 'running', 'succeeded', 'failed', 'cancelled')),
    upstream_task_id TEXT NOT NULL DEFAULT '',
    result_url TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    cost_estimate NUMERIC(12, 6) NOT NULL DEFAULT 0,
    created_by BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_video_tasks_status_updated_at
    ON video_tasks (status, updated_at);
CREATE INDEX IF NOT EXISTS idx_video_tasks_provider_status
    ON video_tasks (provider, status);
CREATE INDEX IF NOT EXISTS idx_video_tasks_created_by_created_at
    ON video_tasks (created_by, created_at DESC);

CREATE TABLE IF NOT EXISTS video_task_events (
    id BIGSERIAL PRIMARY KEY,
    video_task_id BIGINT NOT NULL REFERENCES video_tasks(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    message TEXT NOT NULL DEFAULT '',
    payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_video_task_events_task_created_at
    ON video_task_events (video_task_id, created_at);

CREATE TABLE IF NOT EXISTS video_usage_logs (
    id BIGSERIAL PRIMARY KEY,
    video_task_id BIGINT NOT NULL REFERENCES video_tasks(id) ON DELETE CASCADE,
    provider TEXT NOT NULL CHECK (provider IN ('mock', 'seedance', 'kling')),
    model TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('queued', 'submitted', 'running', 'succeeded', 'failed', 'cancelled')),
    cost_estimate NUMERIC(12, 6) NOT NULL DEFAULT 0,
    duration INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_video_usage_logs_created_at
    ON video_usage_logs (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_video_usage_logs_provider_model
    ON video_usage_logs (provider, model);

INSERT INTO video_provider_accounts (
    provider, display_name, enabled, base_url, default_model, rate_limit_per_minute, metadata_json
) VALUES
    ('mock', 'Mock Provider', TRUE, 'mock://video-gateway', 'mock-video-v1', 120, '{"mode":"local-demo"}'::jsonb),
    ('seedance', 'Seedance 2.0', FALSE, 'https://ark.cn-beijing.volces.com/api/v3', 'seedance-2-0-pro', 60, '{"docs":"https://www.volcengine.com/docs/82379/1520757?lang=zh"}'::jsonb),
    ('kling', 'Kling', FALSE, 'https://api.klingai.com', 'kling-v1', 60, '{"docs":"https://app.klingai.com/cn/dev/document-api/apiReference/updateNotice"}'::jsonb)
ON CONFLICT (provider, display_name) DO NOTHING;
