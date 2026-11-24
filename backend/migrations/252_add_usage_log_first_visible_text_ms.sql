ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS first_visible_text_ms INTEGER;
