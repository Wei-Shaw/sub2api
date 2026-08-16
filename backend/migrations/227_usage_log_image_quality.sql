ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS image_quality VARCHAR(32);
