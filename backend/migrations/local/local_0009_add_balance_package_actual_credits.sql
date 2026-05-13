-- Local extension: computed user-facing actual credit count for balance package cards.
-- The value is derived from package ledger credit and the selected standard balance group's
-- rate_multiplier; it is not manually inserted by UI clients.

ALTER TABLE balance_packages
    ADD COLUMN IF NOT EXISTS actual_credits BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS credit_unit VARCHAR(32) NOT NULL DEFAULT 'credits';

-- Backfill V-Claw balance packages from current package/group parameters.
-- credit_ledger is executable USD ledger balance. A standard group consumes ledger
-- balance at `rate_multiplier`; multiplying by 10,000 turns the effective ledger
-- allowance into user-facing V-Claw credits.
UPDATE balance_packages bp
SET actual_credits = ROUND((bp.credit_ledger / NULLIF(g.rate_multiplier, 0)) * 10000)::BIGINT,
    credit_unit = 'credits'
FROM groups g
WHERE bp.group_id = g.id
  AND g.subscription_type = 'standard'
  AND g.rate_multiplier > 0;
