-- Allow one-shot overdraft on organizations.balance / frozen_balance.
--
-- Background: Settlement runs after the upstream response has already been
-- returned to the client. Pre-flight only knows a tiny minimum-reserve amount,
-- so a company wallet with a small positive balance can pass pre-flight but
-- then fail to cover the real cost at settlement time. The previous CHECK
-- (balance >= 0) constraint made that failure a hard database error, so the
-- request effectively became a free ride: the client got the answer, and no
-- money was deducted.
--
-- New rule: settlement is allowed to drive the balance to a negative value
-- exactly once. The next pre-flight sees balance <= 0 and rejects the request
-- immediately, so we cannot double-overdraft. IAM allocation (users.balance)
-- already has no such CHECK, so this migration only touches organizations.
--
-- Constraints created via `ADD COLUMN ... CHECK (...)` are auto-named
-- <table>_<column>_check by PostgreSQL. Drop them dynamically so we don't
-- have to hard-code the generated name across environments.
DO $$
DECLARE
    r RECORD;
BEGIN
    FOR r IN
        SELECT c.conname
        FROM pg_constraint c
        JOIN pg_class t ON t.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = t.relnamespace
        WHERE t.relname = 'organizations'
          AND n.nspname = current_schema()
          AND c.contype = 'c'
          AND pg_get_constraintdef(c.oid) ~* '\(?\s*balance\s*>=\s*\(?\s*0'
    LOOP
        EXECUTE format('ALTER TABLE organizations DROP CONSTRAINT %I', r.conname);
    END LOOP;
    FOR r IN
        SELECT c.conname
        FROM pg_constraint c
        JOIN pg_class t ON t.oid = c.conrelid
        JOIN pg_namespace n ON n.oid = t.relnamespace
        WHERE t.relname = 'organizations'
          AND n.nspname = current_schema()
          AND c.contype = 'c'
          AND pg_get_constraintdef(c.oid) ~* '\(?\s*frozen_balance\s*>=\s*\(?\s*0'
    LOOP
        EXECUTE format('ALTER TABLE organizations DROP CONSTRAINT %I', r.conname);
    END LOOP;
END $$;
