ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS concurrency_limit BIGINT NOT NULL DEFAULT 0
    CONSTRAINT api_keys_concurrency_limit_nonnegative CHECK (concurrency_limit >= 0);

COMMENT ON COLUMN api_keys.concurrency_limit IS 'API key concurrency limit (0 = no additional limit)';
