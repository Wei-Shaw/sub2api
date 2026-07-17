-- Add a per-key concurrency limit. Existing keys remain unlimited.
ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS concurrency_limit integer NOT NULL DEFAULT 0;
