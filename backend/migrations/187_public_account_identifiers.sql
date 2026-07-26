-- Public account identifiers, phase one. Fields remain nullable until the
-- separately operated backfill and finalize migration have completed.
ALTER TABLE users
    ALTER COLUMN email DROP NOT NULL,
    ADD COLUMN IF NOT EXISTS account_id VARCHAR(16),
    ADD COLUMN IF NOT EXISTS external_user_id VARCHAR(18),
    ADD COLUMN IF NOT EXISTS identity_type VARCHAR(16) NOT NULL DEFAULT 'root',
    ADD COLUMN IF NOT EXISTS login_name VARCHAR(64),
    ADD COLUMN IF NOT EXISTS must_change_password BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS recovery_email VARCHAR(255),
    ADD COLUMN IF NOT EXISTS recovery_email_verified_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS authz_generation BIGINT NOT NULL DEFAULT 1;

DROP INDEX IF EXISTS users_email_unique_active;
CREATE UNIQUE INDEX IF NOT EXISTS users_email_unique_active
    ON users(email)
    WHERE deleted_at IS NULL AND email IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS users_external_user_id_unique_populated
    ON users(external_user_id)
    WHERE external_user_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_users_account_id ON users(account_id);

DO $$ BEGIN
    ALTER TABLE users ADD CONSTRAINT users_identity_type_check
        CHECK (identity_type IN ('root', 'iam'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    ALTER TABLE users ADD CONSTRAINT users_account_id_format_populated_check
        CHECK (account_id IS NULL OR account_id ~ '^[1-9][0-9]{15}$');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    ALTER TABLE users ADD CONSTRAINT users_external_id_format_populated_check
        CHECK (external_user_id IS NULL OR external_user_id ~ '^[1-9][0-9]{15}([0-9]{2})?$');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    ALTER TABLE users ADD CONSTRAINT users_root_identity_populated_check
        CHECK (identity_type <> 'root' OR account_id IS NULL OR external_user_id IS NULL OR account_id = external_user_id);
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    ALTER TABLE users ADD CONSTRAINT users_iam_login_name_check
        CHECK (identity_type <> 'iam' OR login_name ~ '^[A-Za-z0-9._-]{1,64}$');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS users_iam_login_unique_active
    ON users(account_id, LOWER(login_name))
    WHERE identity_type = 'iam' AND deleted_at IS NULL;
