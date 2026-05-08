ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS expose_scheduled_account_in_logs BOOLEAN NOT NULL DEFAULT FALSE;
