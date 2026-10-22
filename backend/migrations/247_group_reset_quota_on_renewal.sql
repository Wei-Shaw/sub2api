-- Opt-in renewal policy; existing groups retain extension-only renewals.
ALTER TABLE groups ADD COLUMN IF NOT EXISTS reset_quota_on_renewal BOOLEAN NOT NULL DEFAULT FALSE;
