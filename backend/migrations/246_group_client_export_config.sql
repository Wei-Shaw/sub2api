-- Defaults only affect newly generated client exports. Existing groups keep
-- native behavior until an administrator explicitly selects a model.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS client_export_config JSONB NOT NULL DEFAULT '{}'::jsonb;
