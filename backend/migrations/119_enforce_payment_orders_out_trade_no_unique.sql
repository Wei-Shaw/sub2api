-- Replace the legacy non-unique index with a partial unique index.
-- Keep empty-string legacy rows compatible while enforcing uniqueness for real order IDs.
DROP INDEX IF EXISTS paymentorder_out_trade_no;

CREATE UNIQUE INDEX IF NOT EXISTS paymentorder_out_trade_no
    ON payment_orders (out_trade_no)
    WHERE out_trade_no <> '';
