-- Add Kiro prompt cache emulation controls to groups.
ALTER TABLE groups
  ADD COLUMN IF NOT EXISTS kiro_cache_emulation_enabled BOOLEAN;

ALTER TABLE groups
  ADD COLUMN IF NOT EXISTS kiro_cache_emulation_ratio DECIMAL(5,4);

-- sub2api-managed-update: reviewed-compatible
UPDATE groups
SET kiro_cache_emulation_enabled = COALESCE(kiro_cache_emulation_enabled, FALSE),
    kiro_cache_emulation_ratio = COALESCE(kiro_cache_emulation_ratio, 1.0)
WHERE kiro_cache_emulation_enabled IS NULL
   OR kiro_cache_emulation_ratio IS NULL;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE groups
  ALTER COLUMN kiro_cache_emulation_enabled SET DEFAULT FALSE;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE groups
  ALTER COLUMN kiro_cache_emulation_enabled SET NOT NULL;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE groups
  ALTER COLUMN kiro_cache_emulation_ratio SET DEFAULT 1.0;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE groups
  ALTER COLUMN kiro_cache_emulation_ratio SET NOT NULL;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE groups
  ADD CONSTRAINT groups_kiro_cache_emulation_ratio_range
  CHECK (kiro_cache_emulation_ratio >= 0 AND kiro_cache_emulation_ratio <= 1);
