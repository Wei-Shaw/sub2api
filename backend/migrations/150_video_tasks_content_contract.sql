-- A-1: persist Seedance content[] contract and billing-relevant video-input flag.
ALTER TABLE video_tasks
    ADD COLUMN IF NOT EXISTS content_json JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE video_tasks
    ADD COLUMN IF NOT EXISTS has_video_input BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE video_tasks
    ADD COLUMN IF NOT EXISTS generate_audio BOOLEAN NULL;

ALTER TABLE video_tasks
    ADD COLUMN IF NOT EXISTS watermark BOOLEAN NULL;

ALTER TABLE video_tasks
    ADD COLUMN IF NOT EXISTS camera_fixed BOOLEAN NULL;

ALTER TABLE video_tasks
    ADD COLUMN IF NOT EXISTS return_last_frame BOOLEAN NULL;

ALTER TABLE video_tasks
    DROP CONSTRAINT IF EXISTS video_tasks_duration_check;

ALTER TABLE video_tasks
    ADD CONSTRAINT video_tasks_duration_check CHECK (duration = -1 OR duration >= 0);
