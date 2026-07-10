DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM (
            SELECT user_id, source_order_id
            FROM user_affiliate_ledger
            WHERE action = 'accrue'
              AND source_order_id IS NOT NULL
            GROUP BY user_id, source_order_id
            HAVING COUNT(*) > 1
        ) duplicate_accruals
    ) THEN
        RAISE EXCEPTION 'duplicate affiliate accrue ledger rows exist for the same user_id and source_order_id';
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_affiliate_ledger_accrue_order_uniq
    ON user_affiliate_ledger(user_id, source_order_id)
    WHERE action = 'accrue' AND source_order_id IS NOT NULL;
