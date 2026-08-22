DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM users
        WHERE account_id IS NULL OR account_id !~ '^[1-9][0-9]{15}$'
    ) THEN
        RAISE EXCEPTION 'account_id verification failed; run the account_id backfill first';
    END IF;
    IF EXISTS (
        SELECT 1 FROM users
        GROUP BY account_id
        HAVING COUNT(*) > 1
    ) THEN
        RAISE EXCEPTION 'duplicate account_id values found; resolve them before finalizing';
    END IF;
END $$;

ALTER TABLE users ALTER COLUMN account_id SET NOT NULL;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_account_id_format_populated_check;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_account_id_format_check;
ALTER TABLE users ADD CONSTRAINT users_account_id_format_check CHECK (account_id ~ '^[1-9][0-9]{15}$');
CREATE UNIQUE INDEX IF NOT EXISTS users_account_id_unique ON users(account_id);
