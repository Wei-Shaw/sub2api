CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_parent_account_id
    ON accounts (parent_account_id) WHERE parent_account_id IS NOT NULL;

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_accounts_spark_shadow_per_parent
    ON accounts (parent_account_id)
    WHERE parent_account_id IS NOT NULL AND quota_dimension = 'spark' AND deleted_at IS NULL;
