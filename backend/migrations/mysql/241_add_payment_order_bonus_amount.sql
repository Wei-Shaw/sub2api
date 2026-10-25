-- MySQL 方言版本（翻译自 ../241_add_payment_order_bonus_amount.sql）
-- 翻译说明：ADD COLUMN IF NOT EXISTS 按迁移执行器单次执行语义去掉修饰词；
-- DECIMAL(20,2) / NOT NULL / DEFAULT 0 原样保留。

-- 充值赠送额度：余额充值订单命中赠送阶梯时的赠送 USD 金额。
-- 已计入 payment_orders.amount（到账总额），单独落列用于订单展示与推广返利基数剔除。
ALTER TABLE payment_orders ADD COLUMN bonus_amount DECIMAL(20,2) NOT NULL DEFAULT 0;
