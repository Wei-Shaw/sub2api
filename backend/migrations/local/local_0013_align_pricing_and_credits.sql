-- Migration: Align group pricing fields + payment_orders.actual_credits
--
-- Consolidates: add group token pricing columns, add payment_orders.actual_credits,
-- update rate_multipliers for all groups, and backfill actual_credits.
--
-- Pricing model (GPT-5.5, 90/10 input/output ratio):
--   token_price_per_million = 0.90 × $5 + 0.10 × $30 = $7.50/1M tokens
--
-- Formula:
--   actual_credits = ledger_amount / rate_multiplier / token_price_per_million × 1,000,000
--   rate_multiplier = amount_ledger / (token_millions × token_price_per_million)
--
-- Balance groups (IDs 14-18):
--   Standard:   $7.34,   27M tokens → R = 7.34 / (27 × 7.50) = 0.036247
--   Pro:        $14.68,  63M tokens → R = 14.68 / (63 × 7.50) = 0.031069
--   Expert:     $29.37, 130M tokens → R = 29.37 / (130 × 7.50) = 0.030123
--   Business:   $73.42, 340M tokens → R = 73.42 / (340 × 7.50) = 0.028792
--   Enterprise: $146.84, 700M tokens → R = 146.84 / (700 × 7.50) = 0.027970
--
-- Subscription groups (via subscription_plans):
--   Basic Pass:  $14.68,   85M tokens → R = 14.68 / (85 × 7.50) = 0.023031
--   Super Pass:  $36.71,  220M tokens → R = 36.71 / (220 × 7.50) = 0.022248
--   Ultra Pass:  $73.42,  450M tokens → R = 73.42 / (450 × 7.50) = 0.021757
--   God Pass:    $146.84, 1000M tokens → R = 146.84 / (1000 × 7.50) = 0.019579

BEGIN;

-- ============================================================
-- PART 1: Add pricing columns to groups
-- ============================================================

ALTER TABLE groups ADD COLUMN IF NOT EXISTS token_price_per_million DECIMAL(20,8) DEFAULT NULL;
ALTER TABLE groups ADD COLUMN IF NOT EXISTS pricing_reference_model VARCHAR(100) DEFAULT NULL;
ALTER TABLE groups ADD COLUMN IF NOT EXISTS input_output_ratio DECIMAL(5,4) DEFAULT NULL;

COMMENT ON COLUMN groups.token_price_per_million IS 'Weighted avg USD price per 1M tokens for USD↔token conversion. NULL = auto-derive from pricing_reference_model.';
COMMENT ON COLUMN groups.pricing_reference_model IS 'Model used as pricing baseline (e.g. gpt-5.5). System resolves input/output prices from model pricing data.';
COMMENT ON COLUMN groups.input_output_ratio IS 'Input token ratio (0.0-1.0) for weighted avg price calculation. 0.90 = 90% input, 10% output. NULL = system default 0.90.';

-- ============================================================
-- PART 2: Add actual_credits to payment_orders
-- ============================================================

ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS actual_credits BIGINT;

CREATE INDEX IF NOT EXISTS idx_payment_orders_actual_credits
  ON payment_orders (user_id, order_type, status)
  WHERE actual_credits IS NOT NULL;

-- ============================================================
-- PART 3: Update balance group rate_multipliers (groups 14-18)
-- ============================================================

WITH balance_data(group_id, amount_ledger, token_millions) AS (
    VALUES
        (14, 7.34::numeric, 27::numeric),
        (15, 14.68::numeric, 63::numeric),
        (16, 29.37::numeric, 130::numeric),
        (17, 73.42::numeric, 340::numeric),
        (18, 146.84::numeric, 700::numeric)
), computed AS (
    SELECT
        group_id,
        ROUND(amount_ledger / (token_millions * 7.50), 8) AS new_rate_multiplier
    FROM balance_data
)
UPDATE groups g
SET rate_multiplier = c.new_rate_multiplier
FROM computed c
WHERE g.id = c.group_id
  AND g.deleted_at IS NULL;

-- ============================================================
-- PART 4: Update subscription group rate_multipliers
-- ============================================================

WITH sub_data(plan_id, price_usd, token_millions) AS (
    VALUES
        (3, 14.68::numeric, 85::numeric),
        (4, 36.71::numeric, 220::numeric),
        (5, 73.42::numeric, 450::numeric),
        (6, 146.84::numeric, 1000::numeric)
), computed AS (
    SELECT
        plan_id,
        ROUND(price_usd / (token_millions * 7.50), 8) AS new_rate_multiplier
    FROM sub_data
)
UPDATE groups g
SET rate_multiplier = c.new_rate_multiplier
FROM computed c
JOIN subscription_plans sp ON sp.id = c.plan_id
WHERE sp.group_id = g.id
  AND g.subscription_type = 'subscription';

-- Set monthly_limit_usd for subscription groups
WITH sub_data(plan_id, price_usd) AS (
    VALUES
        (3, 14.68::numeric),
        (4, 36.71::numeric),
        (5, 73.42::numeric),
        (6, 146.84::numeric)
)
UPDATE groups g
SET monthly_limit_usd = sd.price_usd
FROM sub_data sd
JOIN subscription_plans sp ON sp.id = sd.plan_id
WHERE sp.group_id = g.id
  AND g.subscription_type = 'subscription';

-- ============================================================
-- PART 5: Set pricing fields for all active groups
-- ============================================================

UPDATE groups
SET
    token_price_per_million = 7.50,
    pricing_reference_model = 'gpt-5.5',
    input_output_ratio = 0.90
WHERE deleted_at IS NULL
  AND (token_price_per_million IS NULL OR token_price_per_million <= 0);

-- ============================================================
-- PART 6: Backfill actual_credits for existing orders
-- Uses token_price_per_million = 7.50
-- ============================================================

-- Balance orders
UPDATE payment_orders po
SET actual_credits = ROUND(po.ledger_amount / g.rate_multiplier / 7.50 * 1000000)
FROM groups g
WHERE g.id = po.balance_group_id
  AND po.order_type = 'balance'
  AND po.status = 'COMPLETED'
  AND po.actual_credits IS NULL
  AND g.rate_multiplier > 0;

-- Subscription orders
UPDATE payment_orders po
SET actual_credits = ROUND(po.ledger_amount / g.rate_multiplier / 7.50 * 1000000)
FROM subscription_plans sp
JOIN groups g ON g.id = sp.group_id
WHERE po.plan_id = sp.id
  AND po.order_type = 'subscription'
  AND po.status = 'COMPLETED'
  AND po.actual_credits IS NULL
  AND g.rate_multiplier > 0;

COMMIT;
