-- Local fork migration: bot-sales fulfillment payment order link
-- Downstream extension to link bot_sales_fulfillments with canonical payment_orders

ALTER TABLE bot_sales_fulfillments
  ADD COLUMN IF NOT EXISTS payment_order_id BIGINT
  REFERENCES payment_orders(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS bot_sales_fulfillments_payment_order_id_idx
  ON bot_sales_fulfillments(payment_order_id);
