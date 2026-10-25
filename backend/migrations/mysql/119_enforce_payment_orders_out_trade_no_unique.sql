-- MySQL 方言版本（翻译自 ../119_enforce_payment_orders_out_trade_no_unique.sql）
--
-- 翻译决策：
--   - PG DO 块（pg_indexes 存在性检查 + 条件 DROP/RENAME）→ 临时存储过程，
--     索引存在性检查改用 information_schema.statistics（schemaname='public' → table_schema=DATABASE()）。
--   - 索引名与已翻译的 mysql/120 保持一致：paymentorder_out_trade_no_unique（函数唯一索引）
--     与 paymentorder_out_trade_no（旧普通索引）。
--   - ALTER INDEX ... RENAME TO → ALTER TABLE ... RENAME INDEX ... TO ...（MySQL 8 语法）。

DROP PROCEDURE IF EXISTS __mig_119_tmp;
CREATE PROCEDURE __mig_119_tmp()
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.statistics
        WHERE table_schema = DATABASE()
          AND table_name = 'payment_orders'
          AND index_name = 'paymentorder_out_trade_no_unique'
    ) THEN
        IF EXISTS (
            SELECT 1
            FROM information_schema.statistics
            WHERE table_schema = DATABASE()
              AND table_name = 'payment_orders'
              AND index_name = 'paymentorder_out_trade_no'
        ) THEN
            ALTER TABLE payment_orders DROP INDEX paymentorder_out_trade_no;
        END IF;

        ALTER TABLE payment_orders
            RENAME INDEX paymentorder_out_trade_no_unique TO paymentorder_out_trade_no;
    END IF;
END;

CALL __mig_119_tmp();
DROP PROCEDURE __mig_119_tmp;
