CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_preferred_user_id
    ON accounts (preferred_user_id) WHERE preferred_user_id IS NOT NULL;
