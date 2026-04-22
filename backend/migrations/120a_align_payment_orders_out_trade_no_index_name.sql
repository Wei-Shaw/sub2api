DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_indexes
        WHERE schemaname = 'public'
          AND tablename = 'payment_orders'
          AND indexname = 'paymentorder_out_trade_no_unique'
    ) THEN
        IF EXISTS (
            SELECT 1
            FROM pg_indexes
            WHERE schemaname = 'public'
              AND tablename = 'payment_orders'
              AND indexname = 'paymentorder_out_trade_no'
        ) THEN
            EXECUTE 'DROP INDEX IF EXISTS paymentorder_out_trade_no';
        END IF;

        EXECUTE 'ALTER INDEX paymentorder_out_trade_no_unique RENAME TO paymentorder_out_trade_no';
    END IF;
END $$;
