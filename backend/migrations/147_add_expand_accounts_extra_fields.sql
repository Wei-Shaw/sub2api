ALTER TABLE IF EXISTS expand_accounts
    ADD COLUMN IF NOT EXISTS email_pwd TEXT;

ALTER TABLE IF EXISTS expand_accounts
    ADD COLUMN IF NOT EXISTS help_email TEXT;

ALTER TABLE IF EXISTS expand_accounts
    ADD COLUMN IF NOT EXISTS help_email_url TEXT;

ALTER TABLE IF EXISTS expand_accounts
    ADD COLUMN IF NOT EXISTS channel TEXT;
