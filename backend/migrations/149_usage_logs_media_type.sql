-- Restore usage_logs.media_type for image billing reconciliation.
ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS media_type VARCHAR(16);
