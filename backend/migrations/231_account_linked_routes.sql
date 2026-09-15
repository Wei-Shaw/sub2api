-- Add the generic linked Codex route dimension. Linked routes own routing
-- configuration but resolve OAuth credentials and global quota state through
-- parent_account_id.
ALTER TABLE accounts DROP CONSTRAINT IF EXISTS chk_accounts_quota_dimension;
ALTER TABLE accounts ADD CONSTRAINT chk_accounts_quota_dimension
  CHECK (quota_dimension IN ('global','spark','linked')) NOT VALID;
ALTER TABLE accounts VALIDATE CONSTRAINT chk_accounts_quota_dimension;
