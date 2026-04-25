-- +migrate Up
ALTER TABLE service_quota_rules ADD COLUMN IF NOT EXISTS batch_id uuid NULL;
CREATE INDEX IF NOT EXISTS idx_service_quota_rules_batch_id
    ON service_quota_rules(batch_id) WHERE batch_id IS NOT NULL;

-- +migrate Down
DROP INDEX IF EXISTS idx_service_quota_rules_batch_id;
ALTER TABLE service_quota_rules DROP COLUMN IF EXISTS batch_id;
