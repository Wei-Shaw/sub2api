-- Add Sora platform tables

CREATE TABLE IF NOT EXISTS sora_accounts (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL UNIQUE,
    access_token TEXT,
    session_token TEXT,
    refresh_token TEXT,
    client_id TEXT,
    email TEXT,
    username TEXT,
    remark TEXT,
    use_count INT DEFAULT 0,
    plan_type TEXT,
    plan_title TEXT,
    subscription_end TIMESTAMPTZ,
    sora_supported BOOLEAN DEFAULT FALSE,
    sora_invite_code TEXT,
    sora_redeemed_count INT DEFAULT 0,
    sora_remaining_count INT DEFAULT 0,
    sora_total_count INT DEFAULT 0,
    sora_cooldown_until TIMESTAMPTZ,
    cooled_until TIMESTAMPTZ,
    image_enabled BOOLEAN DEFAULT TRUE,
    video_enabled BOOLEAN DEFAULT TRUE,
    image_concurrency INT DEFAULT -1,
    video_concurrency INT DEFAULT -1,
    is_expired BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (account_id) REFERENCES accounts(id)
);

CREATE INDEX IF NOT EXISTS idx_sora_accounts_plan_type ON sora_accounts (plan_type);
CREATE INDEX IF NOT EXISTS idx_sora_accounts_sora_supported ON sora_accounts (sora_supported);
CREATE INDEX IF NOT EXISTS idx_sora_accounts_image_enabled ON sora_accounts (image_enabled);
CREATE INDEX IF NOT EXISTS idx_sora_accounts_video_enabled ON sora_accounts (video_enabled);

CREATE TABLE IF NOT EXISTS sora_usage_stats (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL UNIQUE,
    image_count INT DEFAULT 0,
    video_count INT DEFAULT 0,
    error_count INT DEFAULT 0,
    last_error_at TIMESTAMPTZ,
    today_image_count INT DEFAULT 0,
    today_video_count INT DEFAULT 0,
    today_error_count INT DEFAULT 0,
    today_date DATE,
    consecutive_error_count INT DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (account_id) REFERENCES accounts(id)
);

CREATE INDEX IF NOT EXISTS idx_sora_usage_stats_today_date ON sora_usage_stats (today_date);

CREATE TABLE IF NOT EXISTS sora_tasks (
    id BIGSERIAL PRIMARY KEY,
    task_id TEXT NOT NULL UNIQUE,
    account_id BIGINT NOT NULL,
    model TEXT NOT NULL,
    prompt TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'processing',
    progress DOUBLE PRECISION DEFAULT 0,
    result_urls TEXT,
    error_message TEXT,
    retry_count INT DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    FOREIGN KEY (account_id) REFERENCES accounts(id)
);

CREATE INDEX IF NOT EXISTS idx_sora_tasks_account_id ON sora_tasks (account_id);
CREATE INDEX IF NOT EXISTS idx_sora_tasks_status ON sora_tasks (status);

CREATE TABLE IF NOT EXISTS sora_cache_files (
    id BIGSERIAL PRIMARY KEY,
    task_id TEXT,
    account_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    media_type TEXT NOT NULL,
    original_url TEXT NOT NULL,
    cache_path TEXT NOT NULL,
    cache_url TEXT NOT NULL,
    size_bytes BIGINT DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (account_id) REFERENCES accounts(id),
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_sora_cache_files_account_id ON sora_cache_files (account_id);
CREATE INDEX IF NOT EXISTS idx_sora_cache_files_user_id ON sora_cache_files (user_id);
CREATE INDEX IF NOT EXISTS idx_sora_cache_files_media_type ON sora_cache_files (media_type);
