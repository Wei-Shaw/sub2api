-- 早期分支版本曾按十分钟时间窗猜测历史管理员充值来源。
-- 只撤销由该已知旧迁移版本写入的历史分类；正式新流水不做时间推断。
WITH superseded_migration AS (
    SELECT applied_at
    FROM schema_migrations
    WHERE filename = '231_affiliate_rebate_sources.sql'
      AND checksum = 'ceb508efbf81877a891a95fe6688cb3287462c2552e1a5c8a8254be9328d6806'
)
UPDATE user_affiliate_ledger ual
SET source_type = 'legacy_unknown',
    base_amount = NULL,
    source_redeem_code_id = NULL,
    updated_at = NOW()
FROM superseded_migration migration
WHERE ual.action = 'accrue'
  AND ual.source_type = 'admin_recharge'
  AND ual.created_at <= migration.applied_at;

-- NOT VALID 避免部署时扫描历史热表；约束仍会立即校验迁移后的新写入。
ALTER TABLE user_affiliate_ledger
    DROP CONSTRAINT IF EXISTS user_affiliate_ledger_source_order_id_fkey;

ALTER TABLE user_affiliate_ledger
    ADD CONSTRAINT user_affiliate_ledger_source_order_id_fkey
    FOREIGN KEY (source_order_id) REFERENCES payment_orders(id) ON DELETE RESTRICT
    NOT VALID;

ALTER TABLE user_affiliate_ledger
    DROP CONSTRAINT IF EXISTS user_affiliate_ledger_source_redeem_code_id_fkey;

ALTER TABLE user_affiliate_ledger
	ADD CONSTRAINT user_affiliate_ledger_source_redeem_code_id_fkey
	FOREIGN KEY (source_redeem_code_id) REFERENCES redeem_codes(id) ON DELETE RESTRICT
    NOT VALID;

ALTER TABLE user_affiliate_ledger
    DROP CONSTRAINT IF EXISTS chk_user_affiliate_ledger_source_type;

ALTER TABLE user_affiliate_ledger
    ADD CONSTRAINT chk_user_affiliate_ledger_source_type CHECK (
        (
            action = 'accrue'
            AND (
                source_type IS NULL
                OR (
                    source_type = 'payment_order'
                    AND source_order_id IS NOT NULL
                    AND source_redeem_code_id IS NULL
                    AND base_amount > 0
                )
                OR (
                    source_type IN ('balance_redeem_code', 'admin_recharge')
                    AND source_order_id IS NULL
                    AND source_redeem_code_id IS NOT NULL
                    AND base_amount > 0
                )
                OR (
                    source_type = 'legacy_unknown'
                    AND source_order_id IS NULL
                    AND source_redeem_code_id IS NULL
                )
            )
        )
        OR (
            action <> 'accrue'
            AND source_type IS NULL
            AND source_order_id IS NULL
            AND source_redeem_code_id IS NULL
            AND base_amount IS NULL
        )
    ) NOT VALID;
