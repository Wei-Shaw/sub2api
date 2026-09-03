-- 索引必须在事务外并发创建，避免阻塞返利流水写入。
DROP INDEX CONCURRENTLY IF EXISTS idx_user_affiliate_ledger_source_type_created_at;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_user_affiliate_ledger_source_type_created_at
    ON user_affiliate_ledger((
        CASE
            WHEN source_order_id IS NOT NULL THEN 'payment_order'
            WHEN source_type IS NOT NULL THEN source_type
            ELSE 'legacy_unknown'
        END
    ), created_at DESC)
    WHERE action = 'accrue';

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_user_affiliate_ledger_accrue_order_uniq
    ON user_affiliate_ledger(source_order_id)
    WHERE action = 'accrue' AND source_order_id IS NOT NULL;

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_user_affiliate_ledger_accrue_redeem_code_uniq
    ON user_affiliate_ledger(source_redeem_code_id)
    WHERE action = 'accrue' AND source_redeem_code_id IS NOT NULL;
