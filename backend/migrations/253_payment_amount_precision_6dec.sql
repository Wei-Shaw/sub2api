-- 充值到账与订单金额精度从 2 位提升到 6 位小数，与余额（decimal(20,8)）和模型价格精度对齐。
-- 仅扩大 numeric scale，PostgreSQL 为元数据变更，不重写表数据。
ALTER TABLE payment_orders ALTER COLUMN amount TYPE DECIMAL(20,6);
ALTER TABLE payment_orders ALTER COLUMN pay_amount TYPE DECIMAL(20,6);
ALTER TABLE payment_orders ALTER COLUMN bonus_amount TYPE DECIMAL(20,6);
ALTER TABLE payment_orders ALTER COLUMN refund_amount TYPE DECIMAL(20,6);

ALTER TABLE subscription_plans ALTER COLUMN price TYPE DECIMAL(20,6);
ALTER TABLE subscription_plans ALTER COLUMN original_price TYPE DECIMAL(20,6);