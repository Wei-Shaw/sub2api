-- 100_add_out_trade_no_to_payment_orders.sql
-- Adds out_trade_no column for external order ID used with payment providers.
-- Allows webhook handlers to look up orders by external ID instead of embedding DB ID.

ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS out_trade_no VARCHAR(64) NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS paymentorder_out_trade_no ON payment_orders (out_trade_no);
