-- Add a per-group switch for Kiro automatic sticky session routing.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS kiro_auto_sticky_enabled BOOLEAN;

-- sub2api-managed-update: reviewed-compatible
UPDATE groups
SET kiro_auto_sticky_enabled = TRUE
WHERE kiro_auto_sticky_enabled IS NULL;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE groups
    ALTER COLUMN kiro_auto_sticky_enabled SET DEFAULT TRUE;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE groups
    ALTER COLUMN kiro_auto_sticky_enabled SET NOT NULL;
