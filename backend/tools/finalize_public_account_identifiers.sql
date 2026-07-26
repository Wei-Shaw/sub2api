DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM users
        WHERE account_id IS NULL OR external_user_id IS NULL
           OR account_id !~ '^[1-9][0-9]{15}$'
           OR external_user_id !~ '^[1-9][0-9]{15}([0-9]{2})?$'
           OR (identity_type = 'root' AND account_id <> external_user_id)
           OR (identity_type = 'iam' AND length(external_user_id) <> 18)
    ) THEN
        RAISE EXCEPTION 'public account identifier verification failed; run the backfill and verification first';
    END IF;
END $$;

DROP INDEX IF EXISTS users_external_user_id_unique_populated;
ALTER TABLE users ALTER COLUMN account_id SET NOT NULL;
ALTER TABLE users ALTER COLUMN external_user_id SET NOT NULL;
ALTER TABLE users ADD CONSTRAINT users_external_user_id_unique UNIQUE (external_user_id);

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_account_id_format_populated_check;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_external_id_format_populated_check;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_root_identity_populated_check;
ALTER TABLE users ADD CONSTRAINT users_account_id_format_check CHECK (account_id ~ '^[1-9][0-9]{15}$');
ALTER TABLE users ADD CONSTRAINT users_external_id_format_check CHECK (
    (identity_type = 'root' AND external_user_id = account_id)
    OR (identity_type = 'iam' AND external_user_id ~ '^[1-9][0-9]{17}$')
);
