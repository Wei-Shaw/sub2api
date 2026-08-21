-- Persist provider image metadata so completed image tasks retain their
-- response shape after a later poll or process restart.
ALTER TABLE async_media_tasks
    ADD COLUMN IF NOT EXISTS image_metadata JSONB NOT NULL DEFAULT '[]'::jsonb;
