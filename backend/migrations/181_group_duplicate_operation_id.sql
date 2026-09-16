-- Persist an internal operation identity for safely recovering an already
-- committed group duplicate when the idempotency response write is ambiguous.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS duplicate_operation_id VARCHAR(64);

CREATE UNIQUE INDEX IF NOT EXISTS idx_groups_duplicate_operation_id_active
    ON groups (duplicate_operation_id)
    WHERE duplicate_operation_id IS NOT NULL AND deleted_at IS NULL;
