CREATE TABLE IF NOT EXISTS prompt_records (
    id BIGSERIAL PRIMARY KEY,
    request_id VARCHAR(128) NOT NULL DEFAULT '',
    turn_no INTEGER NOT NULL DEFAULT 0,
    stage VARCHAR(32) NOT NULL DEFAULT 'http',
    user_id BIGINT,
    username_snapshot VARCHAR(255) NOT NULL DEFAULT '',
    user_email_snapshot VARCHAR(320) NOT NULL DEFAULT '',
    api_key_id BIGINT,
    api_key_name_snapshot VARCHAR(255) NOT NULL DEFAULT '',
    group_id BIGINT,
    group_name VARCHAR(255) NOT NULL DEFAULT '',
    provider VARCHAR(64) NOT NULL DEFAULT '',
    endpoint VARCHAR(128) NOT NULL DEFAULT '',
    protocol VARCHAR(64) NOT NULL DEFAULT '',
    model VARCHAR(255) NOT NULL DEFAULT '',
    prompt_hash CHAR(64) NOT NULL DEFAULT '',
    prompt_text TEXT NOT NULL DEFAULT '',
    prompt_length INTEGER NOT NULL DEFAULT 0,
    message_count INTEGER NOT NULL DEFAULT 0,
    risk_status VARCHAR(32) NOT NULL DEFAULT 'pending',
    risk_result JSONB NOT NULL DEFAULT '{}'::jsonb,
    risk_checked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_prompt_records_created_at ON prompt_records (created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_prompt_records_user_created ON prompt_records (user_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_prompt_records_api_key_created ON prompt_records (api_key_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_prompt_records_hash ON prompt_records (prompt_hash);
CREATE UNIQUE INDEX IF NOT EXISTS uq_prompt_records_dedupe
    ON prompt_records (request_id, stage, turn_no, prompt_hash, COALESCE(api_key_id, 0));
