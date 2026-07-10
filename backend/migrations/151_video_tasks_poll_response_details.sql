ALTER TABLE video_tasks
    ADD COLUMN IF NOT EXISTS usage_total_tokens BIGINT,
    ADD COLUMN IF NOT EXISTS actual_resolution VARCHAR(32),
    ADD COLUMN IF NOT EXISTS actual_duration INTEGER,
    ADD COLUMN IF NOT EXISTS last_frame_url TEXT;
