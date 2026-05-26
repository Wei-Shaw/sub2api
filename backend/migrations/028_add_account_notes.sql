-- 028_add_account_notes.sql
-- Add optional admin notes for accounts.

ALTER TABLE accounts
ADD COLUMN IF NOT EXISTS notes TEXT;

COMMENT ON COLUMN accounts.notes IS 'Admin-only notes for account';
