ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS first_output_failover_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS first_output_failover_timeout_seconds INTEGER NOT NULL DEFAULT 6,
    ADD COLUMN IF NOT EXISTS first_output_failover_max_switches INTEGER NOT NULL DEFAULT 3;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'groups_first_output_failover_timeout_seconds_check'
          AND conrelid = 'groups'::regclass
    ) THEN
        ALTER TABLE groups
            ADD CONSTRAINT groups_first_output_failover_timeout_seconds_check
            CHECK (first_output_failover_timeout_seconds BETWEEN 1 AND 600);
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'groups_first_output_failover_max_switches_check'
          AND conrelid = 'groups'::regclass
    ) THEN
        ALTER TABLE groups
            ADD CONSTRAINT groups_first_output_failover_max_switches_check
            CHECK (first_output_failover_max_switches BETWEEN 1 AND 10);
    END IF;
END $$;
