-- 为邀请返利流水补充统一来源字段。本迁移只做快速加列，避免服务启动时扫描和更新整张资金流水表。
ALTER TABLE user_affiliate_ledger
    ADD COLUMN IF NOT EXISTS source_type VARCHAR(32) NULL;

ALTER TABLE user_affiliate_ledger
    ADD COLUMN IF NOT EXISTS base_amount DECIMAL(20,8) NULL;

ALTER TABLE user_affiliate_ledger
    ADD COLUMN IF NOT EXISTS source_redeem_code_id BIGINT NULL;

COMMENT ON COLUMN user_affiliate_ledger.source_type IS '返利来源：payment_order|balance_redeem_code|admin_recharge|legacy_unknown；历史未分类流水为 NULL';
COMMENT ON COLUMN user_affiliate_ledger.base_amount IS '计算该笔返利时使用的充值金额快照';
COMMENT ON COLUMN user_affiliate_ledger.source_redeem_code_id IS '产生返利的余额兑换码或管理员余额调整记录';
