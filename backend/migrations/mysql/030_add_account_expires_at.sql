-- MySQL 方言版本（翻译自 ../030_add_account_expires_at.sql）

-- Add expires_at for account expiration configuration
-- PG timestamptz（可空）→ DATETIME(3) NULL
ALTER TABLE accounts ADD COLUMN expires_at DATETIME(3) NULL;
-- Document expires_at meaning
-- (comment skipped)
-- Add auto_pause_on_expired for account expiration scheduling control
ALTER TABLE accounts ADD COLUMN auto_pause_on_expired BOOLEAN NOT NULL DEFAULT TRUE;
-- Document auto_pause_on_expired meaning
-- (comment skipped)
-- Ensure existing accounts are enabled by default
UPDATE accounts SET auto_pause_on_expired = TRUE;
