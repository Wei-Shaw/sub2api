-- 154_account_spark_shadow.sql
ALTER TABLE accounts
    ADD COLUMN IF NOT EXISTS parent_account_id BIGINT,
    ADD COLUMN IF NOT EXISTS quota_dimension VARCHAR(20) NOT NULL DEFAULT 'global';

-- 幂等加约束:维度合法 + 禁自指 + parent⟺非global 维度一致(评审 P1-d)
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_accounts_quota_dimension') THEN
    ALTER TABLE accounts ADD CONSTRAINT chk_accounts_quota_dimension
      CHECK (quota_dimension IN ('global','spark')) NOT VALID;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_accounts_parent_dimension') THEN
    ALTER TABLE accounts ADD CONSTRAINT chk_accounts_parent_dimension
      CHECK ((parent_account_id IS NULL AND quota_dimension = 'global')
          OR (parent_account_id IS NOT NULL AND quota_dimension <> 'global')) NOT VALID;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_accounts_parent_not_self') THEN
    ALTER TABLE accounts ADD CONSTRAINT chk_accounts_parent_not_self
      CHECK (parent_account_id IS NULL OR parent_account_id <> id) NOT VALID;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_accounts_parent_account_id') THEN
    ALTER TABLE accounts ADD CONSTRAINT fk_accounts_parent_account_id
      FOREIGN KEY (parent_account_id) REFERENCES accounts(id) ON DELETE RESTRICT NOT VALID;
  END IF;
END $$;

ALTER TABLE accounts VALIDATE CONSTRAINT chk_accounts_quota_dimension;
ALTER TABLE accounts VALIDATE CONSTRAINT chk_accounts_parent_dimension;
ALTER TABLE accounts VALIDATE CONSTRAINT chk_accounts_parent_not_self;
ALTER TABLE accounts VALIDATE CONSTRAINT fk_accounts_parent_account_id;
