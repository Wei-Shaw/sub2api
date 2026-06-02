-- Recalculate V-Claw balance credit display snapshots with the approved
-- rate=1 package-credit model:
--   amount_ledger = token_millions * token_price_per_million
--   tokens = amount_ledger / rate_multiplier / token_price_per_million * 1,000,000
-- Balance-package groups use rate_multiplier=1 so package ledger balance directly
-- represents the USD-equivalent token budget. The package rows keep currency
-- overrides for sale/collection price (USD/CNY/VND), so checkout can charge the
-- configured retail amount while fulfillment credits the larger ledger budget.
-- Runtime usage burn remains upstream: ActualCost = provider TotalCost *
-- rate_multiplier, and top-ups credit users.balance by the stored ledger_amount.

ALTER TABLE groups
    ALTER COLUMN rate_multiplier TYPE DECIMAL(20,8) USING rate_multiplier::numeric;

WITH balance_groups(group_id) AS (
    VALUES (14), (15), (16), (17), (18)
)
UPDATE groups g
SET rate_multiplier = 1,
    token_price_per_million = 7.50,
    pricing_reference_model = 'gpt-5.5',
    input_output_ratio = 0.90
FROM balance_groups bg
WHERE g.id = bg.group_id
  AND g.deleted_at IS NULL;

WITH balance_data(group_id, code, amount_ledger, token_millions) AS (
    VALUES
        (14, 'standard',   202.50::numeric, 27::numeric),
        (15, 'pro',        472.50::numeric, 63::numeric),
        (16, 'expert',     975.00::numeric, 130::numeric),
        (17, 'business',   2550.00::numeric, 340::numeric),
        (18, 'enterprise', 5250.00::numeric, 700::numeric)
)
UPDATE balance_packages bp
SET amount_ledger = c.amount_ledger,
    actual_credits = (c.token_millions * 1000000)::bigint,
    credit_unit = 'tokens',
    currency_overrides = jsonb_build_object(
        'USD', CASE c.code
            WHEN 'standard' THEN 7.34
            WHEN 'pro' THEN 14.68
            WHEN 'expert' THEN 29.37
            WHEN 'business' THEN 73.42
            WHEN 'enterprise' THEN 146.84
        END,
        'CNY', CASE c.code
            WHEN 'standard' THEN 49.99
            WHEN 'pro' THEN 99.97
            WHEN 'expert' THEN 199.99
            WHEN 'business' THEN 499.99
            WHEN 'enterprise' THEN 1000.00
        END,
        'VND', CASE c.code
            WHEN 'standard' THEN 187170
            WHEN 'pro' THEN 374340
            WHEN 'expert' THEN 749935
            WHEN 'business' THEN 1872210
            WHEN 'enterprise' THEN 3744420
        END
    )
FROM balance_data c
JOIN groups g ON g.id = c.group_id
WHERE bp.code = c.code
  AND bp.group_id = c.group_id
  AND g.rate_multiplier > 0
  AND g.token_price_per_million > 0;

WITH balance_groups(group_id) AS (
    VALUES (14), (15), (16), (17), (18)
)
UPDATE payment_orders po
SET actual_credits = ROUND(po.ledger_amount / g.rate_multiplier / NULLIF(g.token_price_per_million, 0) * 1000000)::bigint
FROM balance_groups bg
JOIN groups g ON g.id = bg.group_id
WHERE g.id = po.balance_group_id
  AND po.order_type = 'balance'
  AND po.status = 'COMPLETED'
  AND po.ledger_amount > 0
  AND g.rate_multiplier > 0
  AND g.token_price_per_million > 0;
