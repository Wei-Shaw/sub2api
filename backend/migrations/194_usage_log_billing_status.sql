BEGIN;

ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS billing_status VARCHAR(16) NOT NULL DEFAULT 'settled';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'usage_logs_billing_status_check'
          AND conrelid = 'usage_logs'::regclass
    ) THEN
        ALTER TABLE usage_logs
            ADD CONSTRAINT usage_logs_billing_status_check
            CHECK (billing_status IN ('settled', 'unsettled'));
    END IF;
END;
$$;

COMMIT;
