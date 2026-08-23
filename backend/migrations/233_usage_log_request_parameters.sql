-- Preserve the sanitized client request parameters for usage detail views.
ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS request_parameters JSONB;

ALTER TABLE async_media_tasks
    ADD COLUMN IF NOT EXISTS request_parameters JSONB;
