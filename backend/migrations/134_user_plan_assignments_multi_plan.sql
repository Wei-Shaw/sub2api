-- Allow a user to hold multiple benefit plans simultaneously.
-- Move user_plan_assignments primary key from (user_id) to (user_id, plan_id).

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'user_plan_assignments'::regclass
          AND conname = 'user_plan_assignments_pkey'
    ) THEN
        ALTER TABLE user_plan_assignments
            DROP CONSTRAINT user_plan_assignments_pkey;
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'user_plan_assignments'::regclass
          AND conname = 'user_plan_assignments_user_id_plan_id_pkey'
    ) THEN
        ALTER TABLE user_plan_assignments
            ADD CONSTRAINT user_plan_assignments_user_id_plan_id_pkey PRIMARY KEY (user_id, plan_id);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_user_plan_assignments_user_id
    ON user_plan_assignments (user_id);
