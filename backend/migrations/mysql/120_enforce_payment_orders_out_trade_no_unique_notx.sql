-- MySQL 方言版本（翻译自 ../120_enforce_payment_orders_out_trade_no_unique_notx.sql）
-- Build the payment order uniqueness guarantee online.
-- （PG 版使用 CONCURRENTLY 在线建部分唯一索引 WHERE out_trade_no <> ''；
--   MySQL 无 CONCURRENTLY，改为函数唯一索引键部分 IF(out_trade_no <> '', 0, NULL)。）

CREATE UNIQUE INDEX paymentorder_out_trade_no_unique
    ON payment_orders (out_trade_no, (IF(out_trade_no <> '', 0, NULL)));

-- 移除旧的普通索引（PG: DROP INDEX CONCURRENTLY IF EXISTS paymentorder_out_trade_no）
ALTER TABLE payment_orders DROP INDEX paymentorder_out_trade_no;
