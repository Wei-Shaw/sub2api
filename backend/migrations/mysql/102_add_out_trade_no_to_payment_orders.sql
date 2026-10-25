-- MySQL 方言版本（翻译自 ../102_add_out_trade_no_to_payment_orders.sql）
-- 翻译说明：CREATE INDEX 的 IF NOT EXISTS 按规则去掉（执行器保证单次执行）。
-- （后续 120_enforce_payment_orders_out_trade_no_unique_notx 会以函数唯一索引
--   替换本普通索引并 DROP 同名索引，名称保持一致。）

-- Adds out_trade_no column for external order ID used with payment providers.
-- Allows webhook handlers to look up orders by external ID instead of embedding DB ID.
ALTER TABLE payment_orders ADD COLUMN out_trade_no VARCHAR(64) NOT NULL DEFAULT '';
CREATE INDEX paymentorder_out_trade_no ON payment_orders (out_trade_no);
