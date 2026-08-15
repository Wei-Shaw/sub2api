-- Refresh the description of finance-related managed policies to reflect that
-- they now also grant access to the organization dashboard and usage records.
-- The underlying action set stays untouched; this migration only rewrites the
-- human-readable description shown in the console.

UPDATE managed_policies
SET description = 'View the root account available, frozen, and total balance, plus the organization dashboard and usage records.'
WHERE policy_key = 'CompanyFinanceReadOnly';

UPDATE managed_policies
SET description = 'Read root balance, view the organization dashboard and usage records, allocate/reclaim member balance, manage member spend limits, and purchase or cancel company subscription plans.'
WHERE policy_key = 'CompanyFinanceManage';
