-- Kiro group-level inference endpoint selector.
-- q   = AWS Q (q.{region}.amazonaws.com)  [default, backfills existing rows]
-- krs = Kiro Runtime Service (runtime.us-east-1.kiro.dev)
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS kiro_endpoint_mode VARCHAR(8);

-- sub2api-managed-update: reviewed-compatible
UPDATE groups
SET kiro_endpoint_mode = 'q'
WHERE kiro_endpoint_mode IS NULL;

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE groups
    ALTER COLUMN kiro_endpoint_mode SET DEFAULT 'q';

-- sub2api-managed-update: reviewed-compatible
ALTER TABLE groups
    ALTER COLUMN kiro_endpoint_mode SET NOT NULL;
