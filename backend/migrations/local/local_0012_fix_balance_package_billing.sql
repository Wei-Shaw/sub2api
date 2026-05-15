-- Fix balance package billing: correct rate_multiplier and deprecate unused fields.
--
-- Previously rate_multiplier was 0.175 for all groups, which is incorrect.
-- Correct values computed from GPT-5.5 pricing:
--   avg_price = (3×$2.5 + 1×$15) / 4 = $5.625/1M tokens
--   rate_multiplier = amount_ledger / (actual_credits / 1M × avg_price)
--
-- After this migration:
--   user pays $7.34 → balance += $7.34
--   billing: ActualCost = TotalCost × rate_multiplier
--   $7.34 lasts exactly 27M tokens (Standard package)
--
-- credit_ledger, bonus_ledger, credit_multiplier are deprecated:
--   - credit_ledger = amount_ledger (no inflated balance)
--   - bonus_ledger = 0
--   - credit_multiplier = 1
--   These columns remain for backward compat but are not used in billing.

-- Fix rate_multiplier for balance package groups
UPDATE groups SET rate_multiplier = 0.048329 WHERE id = 14; -- Standard: $7.34 → 27M tokens
UPDATE groups SET rate_multiplier = 0.041425 WHERE id = 15; -- Pro: $14.68 → 63M tokens
UPDATE groups SET rate_multiplier = 0.040164 WHERE id = 16; -- Expert: $29.37 → 130M tokens
UPDATE groups SET rate_multiplier = 0.038390 WHERE id = 17; -- Business: $73.42 → 340M tokens
UPDATE groups SET rate_multiplier = 0.037293 WHERE id = 18; -- Enterprise: $146.84 → 700M tokens

-- Deprecate unused fields (keep columns, reset values)
UPDATE balance_packages SET credit_ledger = amount_ledger, bonus_ledger = 0, credit_multiplier = 1;

-- Update credit_unit to 'tokens' for clarity
UPDATE balance_packages SET credit_unit = 'tokens' WHERE credit_unit = 'credits';

-- Drop deprecated columns (no longer used in code)
ALTER TABLE balance_packages DROP COLUMN IF EXISTS credit_ledger;
ALTER TABLE balance_packages DROP COLUMN IF EXISTS bonus_ledger;
ALTER TABLE balance_packages DROP COLUMN IF EXISTS credit_multiplier;
