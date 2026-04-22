-- Build the payment order uniqueness guarantee online.
-- Create the new partial unique index concurrently first so writes keep flowing,
-- then remove the legacy index name once the replacement is ready.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS paymentorder_out_trade_no_unique
    ON payment_orders (out_trade_no)
    WHERE out_trade_no <> '';

DROP INDEX CONCURRENTLY IF EXISTS paymentorder_out_trade_no;
