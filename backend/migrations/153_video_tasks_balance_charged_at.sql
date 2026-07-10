ALTER TABLE video_tasks
    ADD COLUMN IF NOT EXISTS balance_charged_at TIMESTAMPTZ;
