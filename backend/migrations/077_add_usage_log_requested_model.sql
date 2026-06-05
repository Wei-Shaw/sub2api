-- Add requested_model field to usage_logs for normalized request/upstream model tracking.
-- NULL means historical rows written before requested_model dual-write was introduced.
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS requested_model VARCHAR(100);
