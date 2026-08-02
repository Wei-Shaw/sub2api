ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS organization_id BIGINT,
    ADD COLUMN IF NOT EXISTS payer_user_id BIGINT,
    ADD COLUMN IF NOT EXISTS balance_source VARCHAR(16),
    ADD COLUMN IF NOT EXISTS authz_generation BIGINT;
CREATE INDEX IF NOT EXISTS idx_usage_logs_organization_created_at
    ON usage_logs(organization_id, created_at) WHERE organization_id IS NOT NULL;

ALTER TABLE balance_ledger
    ADD COLUMN IF NOT EXISTS organization_id BIGINT,
    ADD COLUMN IF NOT EXISTS payer_user_id BIGINT,
    ADD COLUMN IF NOT EXISTS balance_source VARCHAR(16),
    ADD COLUMN IF NOT EXISTS authz_generation BIGINT;
CREATE INDEX IF NOT EXISTS idx_balance_ledger_payer_created
    ON balance_ledger(payer_user_id, created_at) WHERE payer_user_id IS NOT NULL;

ALTER TABLE async_media_tasks
    ADD COLUMN IF NOT EXISTS organization_id BIGINT,
    ADD COLUMN IF NOT EXISTS payer_user_id BIGINT,
    ADD COLUMN IF NOT EXISTS balance_source VARCHAR(16),
    ADD COLUMN IF NOT EXISTS authz_generation BIGINT;
CREATE INDEX IF NOT EXISTS idx_async_media_org_created
    ON async_media_tasks(organization_id, created_at) WHERE organization_id IS NOT NULL;

ALTER TABLE batch_image_jobs
    ADD COLUMN IF NOT EXISTS organization_id BIGINT,
    ADD COLUMN IF NOT EXISTS payer_user_id BIGINT,
    ADD COLUMN IF NOT EXISTS balance_source VARCHAR(16),
    ADD COLUMN IF NOT EXISTS authz_generation BIGINT;
CREATE INDEX IF NOT EXISTS idx_batch_image_org_created
    ON batch_image_jobs(organization_id, created_at) WHERE organization_id IS NOT NULL;
