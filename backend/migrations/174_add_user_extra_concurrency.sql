-- Existing users keep their current standard concurrency and receive no extra quota.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS extra_concurrency INTEGER NOT NULL DEFAULT 0;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'users_extra_concurrency_nonnegative'
          AND conrelid = 'users'::regclass
    ) THEN
        ALTER TABLE users
            ADD CONSTRAINT users_extra_concurrency_nonnegative
            CHECK (extra_concurrency >= 0);
    END IF;
END $$;

ALTER TABLE redeem_codes
    ALTER COLUMN type TYPE VARCHAR(32);
