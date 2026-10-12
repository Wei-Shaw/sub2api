-- Keep the acceptance record independent of short-lived Redis state. An unknown
-- submission must never be reclaimed automatically: the provider may be billing it.
CREATE TABLE IF NOT EXISTS video_submissions (
    api_key_id BIGINT NOT NULL,
    key_hash TEXT NOT NULL,
    payload_hash TEXT NOT NULL,
    task_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (api_key_id, key_hash)
);
