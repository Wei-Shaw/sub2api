ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS concurrency_limit INTEGER NOT NULL DEFAULT 0
    CHECK (concurrency_limit >= 0);
