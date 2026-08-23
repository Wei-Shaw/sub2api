-- Make account_id the sole public user identifier.
-- IAM users are associated with organizations through organization_memberships;
-- they must not reuse the organization's/root user's account_id.

-- A duplicate root account cannot be resolved automatically because organizations
-- and historical billing data may refer to it. Abort before changing any data.
DO $$
DECLARE
    duplicate_account RECORD;
    root_count INTEGER;
BEGIN
    FOR duplicate_account IN
        SELECT account_id
        FROM users
        WHERE account_id IS NOT NULL
        GROUP BY account_id
        HAVING COUNT(*) > 1
    LOOP
        SELECT COUNT(*) INTO root_count
        FROM users
        WHERE account_id = duplicate_account.account_id
          AND identity_type = 'root';
        IF root_count > 1 THEN
            RAISE EXCEPTION
                'cannot migrate account_id %, multiple root users share this identifier',
                duplicate_account.account_id;
        END IF;
    END LOOP;
END $$;

-- Assign fresh 16-digit identifiers only to IAM users that still share an ID
-- and rows with no ID. The candidate is derived from the immutable row id only
-- to make retries deterministic; collisions with existing identifiers are
-- handled by the loop below.
DO $$
DECLARE
    item RECORD;
    candidate TEXT;
    attempt INTEGER;
BEGIN
    FOR item IN
        SELECT id
        FROM users
        WHERE account_id IS NULL
           OR (
               identity_type = 'iam'
               AND EXISTS (
                   SELECT 1 FROM users other
                   WHERE other.account_id = users.account_id
                     AND other.id <> users.id
               )
           )
        ORDER BY id
    LOOP
        attempt := 0;
        LOOP
            candidate := '9' || LPAD(((item.id + attempt) % 1000000000000000)::TEXT, 15, '0');
            EXIT WHEN NOT EXISTS (
                SELECT 1 FROM users
                WHERE account_id = candidate AND id <> item.id
            );
            attempt := attempt + 1;
            IF attempt > 1000000 THEN
                RAISE EXCEPTION 'unable to allocate a unique account_id for user %', item.id;
            END IF;
        END LOOP;
        UPDATE users SET account_id = candidate WHERE id = item.id;
    END LOOP;
END $$;

ALTER TABLE users ALTER COLUMN account_id SET NOT NULL;

DROP INDEX IF EXISTS idx_users_account_id;
DROP INDEX IF EXISTS users_iam_login_unique_active;
DROP INDEX IF EXISTS users_external_user_id_unique_populated;
DROP INDEX IF EXISTS user_external_user_id;

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_account_id_format_populated_check;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_external_id_format_populated_check;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_root_identity_populated_check;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_account_id_format_check;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_external_id_format_check;

ALTER TABLE users
    ADD CONSTRAINT users_account_id_format_check
    CHECK (account_id ~ '^[1-9][0-9]{15}$');

CREATE UNIQUE INDEX IF NOT EXISTS users_account_id_unique ON users(account_id);

ALTER TABLE users DROP COLUMN IF EXISTS external_user_id;
