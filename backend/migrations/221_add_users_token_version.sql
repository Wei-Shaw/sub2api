-- Persist the per-user security stamp used by access and refresh tokens.
--
-- Existing deployments intentionally start at version 0. Tokens issued before
-- this migration used an email/password fingerprint instead, so they no longer
-- match and every existing session must authenticate once after the upgrade.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS token_version BIGINT NOT NULL DEFAULT 0;

-- Repair a partially-created column as well as creating it from scratch.
ALTER TABLE users
    ALTER COLUMN token_version SET DEFAULT 0;
UPDATE users SET token_version = 0 WHERE token_version IS NULL;
ALTER TABLE users
    ALTER COLUMN token_version SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'users_token_version_nonnegative'
          AND conrelid = 'users'::regclass
    ) THEN
        ALTER TABLE users
            ADD CONSTRAINT users_token_version_nonnegative
            CHECK (token_version >= 0);
    END IF;
END
$$;

COMMENT ON COLUMN users.token_version IS
    'Persistent JWT/refresh-token security stamp; atomically incremented to revoke all sessions.';
