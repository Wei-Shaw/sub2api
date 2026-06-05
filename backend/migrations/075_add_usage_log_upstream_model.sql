-- Add upstream_model field to usage_logs.
-- Stores the actual upstream model name when it differs from the requested model
-- (i.e., when model mapping is applied). NULL means no mapping was applied.
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS upstream_model VARCHAR(100);
