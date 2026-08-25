-- Split Kiro prompt-cache emulation ratios while preserving existing behavior.
ALTER TABLE groups
  ADD COLUMN IF NOT EXISTS kiro_cache_emulation_mode VARCHAR(16);

ALTER TABLE groups
  ADD COLUMN IF NOT EXISTS kiro_cache_creation_emulation_ratio DECIMAL(5,4);

ALTER TABLE groups
  ADD COLUMN IF NOT EXISTS kiro_cache_read_emulation_ratio DECIMAL(5,4);

-- sub2api-managed-update: reviewed-compatible
UPDATE groups
SET
  kiro_cache_emulation_mode = COALESCE(kiro_cache_emulation_mode, 'uniform'),
  kiro_cache_creation_emulation_ratio = kiro_cache_emulation_ratio,
  kiro_cache_read_emulation_ratio = kiro_cache_emulation_ratio
WHERE kiro_cache_creation_emulation_ratio IS NULL
   OR kiro_cache_read_emulation_ratio IS NULL
   OR kiro_cache_emulation_mode IS NULL;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE groups
  ALTER COLUMN kiro_cache_emulation_mode SET DEFAULT 'uniform';

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE groups
  ALTER COLUMN kiro_cache_emulation_mode SET NOT NULL;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE groups
  ALTER COLUMN kiro_cache_creation_emulation_ratio SET DEFAULT 1.0;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE groups
  ALTER COLUMN kiro_cache_creation_emulation_ratio SET NOT NULL;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE groups
  ALTER COLUMN kiro_cache_read_emulation_ratio SET DEFAULT 1.0;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE groups
  ALTER COLUMN kiro_cache_read_emulation_ratio SET NOT NULL;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE groups
  ADD CONSTRAINT groups_kiro_cache_emulation_mode_valid
  CHECK (kiro_cache_emulation_mode IN ('uniform', 'independent'));

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE groups
  ADD CONSTRAINT groups_kiro_cache_creation_emulation_ratio_range
  CHECK (kiro_cache_creation_emulation_ratio >= 0 AND kiro_cache_creation_emulation_ratio <= 1);

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE groups
  ADD CONSTRAINT groups_kiro_cache_read_emulation_ratio_range
  CHECK (kiro_cache_read_emulation_ratio >= 0 AND kiro_cache_read_emulation_ratio <= 1);
