-- Add a per-group switch for Kiro automatic sticky session routing.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS kiro_auto_sticky_enabled BOOLEAN NOT NULL DEFAULT TRUE;
