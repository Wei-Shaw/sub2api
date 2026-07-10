ALTER TABLE video_tasks
    ADD COLUMN IF NOT EXISTS local_asset_path TEXT,
    ADD COLUMN IF NOT EXISTS local_asset_saved_at TIMESTAMPTZ;
