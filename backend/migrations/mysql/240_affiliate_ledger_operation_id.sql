-- MySQL 方言版本（翻译自 ../240_affiliate_ledger_operation_id.sql）
-- 管理员登记线下提现的幂等标识：同一 operation_id 只对应一条流水，
-- 重试同一次登记时据此返回首次登记的结果，不重复扣减。
--
-- 说明：PG 版部分唯一索引 WHERE operation_id IS NOT NULL →
--       MySQL 唯一索引天然允许多个 NULL，直接去掉 WHERE 谓词。

ALTER TABLE user_affiliate_ledger
    ADD COLUMN operation_id VARCHAR(64) NULL;

-- (comment skipped: user_affiliate_ledger.operation_id)

CREATE UNIQUE INDEX idx_user_affiliate_ledger_operation_id
    ON user_affiliate_ledger (operation_id);
