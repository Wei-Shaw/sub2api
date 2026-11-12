SET LOCAL lock_timeout = '5s';

ALTER TABLE accounts ADD COLUMN IF NOT EXISTS preferred_user_id BIGINT;
ALTER TABLE groups ADD COLUMN IF NOT EXISTS prefer_user_accounts BOOLEAN NOT NULL DEFAULT FALSE;

-- Enforce new writes immediately. Validate historical rows after releasing
-- the stronger locks taken by ADD COLUMN.
ALTER TABLE accounts
    ADD CONSTRAINT fk_accounts_preferred_user_id
    FOREIGN KEY (preferred_user_id) REFERENCES users(id) ON DELETE SET NULL NOT VALID;
