-- local_0014_currency_overrides.sql
-- Add per-currency price overrides to subscription_plans and balance_packages.
-- When set, the override amount is charged to the user instead of FX-converting
-- the ledger price. Billing (ledger_amount, actual_credits) remains unchanged.

ALTER TABLE subscription_plans
    ADD COLUMN IF NOT EXISTS currency_overrides jsonb DEFAULT '{}';

ALTER TABLE balance_packages
    ADD COLUMN IF NOT EXISTS currency_overrides jsonb DEFAULT '{}';

COMMENT ON COLUMN subscription_plans.currency_overrides IS
    'Per-currency display/payment price overrides; key=ISO currency code, value=amount in that currency. When set, this amount is charged instead of FX-converting the ledger price.';

COMMENT ON COLUMN balance_packages.currency_overrides IS
    'Per-currency display/payment price overrides; key=ISO currency code, value=amount in that currency. When set, this amount is charged instead of FX-converting the ledger amount.';
