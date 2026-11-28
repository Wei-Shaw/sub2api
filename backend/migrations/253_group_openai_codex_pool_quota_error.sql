ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS openai_codex_pool_quota_error_enabled BOOLEAN NOT NULL DEFAULT FALSE;
