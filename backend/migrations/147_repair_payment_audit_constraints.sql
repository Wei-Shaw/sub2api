-- Repair payment_audit_logs constraints after legacy/imported tables that were
-- created without the primary key and order/action idempotency index.
WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (PARTITION BY order_id, action ORDER BY id) AS rn
    FROM payment_audit_logs
)
DELETE FROM payment_audit_logs p
USING ranked r
WHERE p.id = r.id
  AND r.rn > 1;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'payment_audit_logs'::regclass
          AND contype = 'p'
    ) THEN
        ALTER TABLE payment_audit_logs
            ADD CONSTRAINT payment_audit_logs_pkey PRIMARY KEY (id);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_payment_audit_logs_order_id
    ON payment_audit_logs(order_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_payment_audit_logs_order_action_uniq
    ON payment_audit_logs(order_id, action);
