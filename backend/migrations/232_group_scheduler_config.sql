-- Additive per-group scheduler policy. Empty JSON preserves legacy behavior.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS scheduler_config JSONB NOT NULL DEFAULT '{}'::jsonb;
