-- Company self-owned balance.
--
-- Until now a company's spendable balance was effectively the owner (root)
-- user's users.balance. This gives every organization its own balance /
-- frozen_balance so that company API keys can consume company funds
-- independently from the owner's personal balance. A finance-privileged account
-- (currently the owner) moves money from their personal balance into the
-- company balance.
ALTER TABLE organizations
    ADD COLUMN IF NOT EXISTS balance NUMERIC(20,8) NOT NULL DEFAULT 0 CHECK (balance >= 0),
    ADD COLUMN IF NOT EXISTS frozen_balance NUMERIC(20,8) NOT NULL DEFAULT 0 CHECK (frozen_balance >= 0);

-- Company top-ups / withdrawals are recorded in the existing financial ledger.
-- The funds move between a user's personal balance and the company balance, so
-- exactly one of source_user_id / destination_user_id is NULL for these kinds.
ALTER TABLE organization_financial_ledger
    DROP CONSTRAINT IF EXISTS organization_financial_ledger_kind_check;
ALTER TABLE organization_financial_ledger
    ADD CONSTRAINT organization_financial_ledger_kind_check
    CHECK (kind IN ('upgrade_reserve', 'upgrade_capture', 'upgrade_release', 'allocate', 'reclaim', 'company_deposit', 'company_withdraw'));
