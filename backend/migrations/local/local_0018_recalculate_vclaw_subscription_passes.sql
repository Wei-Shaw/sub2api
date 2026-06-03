-- Local correction: repair V-Claw monthly subscription pass multipliers.
--
-- Runtime contract for subscription quotas:
--   actual_subscription_usage_usd = raw_provider_cost_usd * groups.rate_multiplier
--   user_subscriptions.monthly_usage_usd += actual_subscription_usage_usd
--
-- For these passes the public monthly price is also the quota ledger limit, and
-- the advertised token-equivalent quota is derived from GPT-5.5 weighted token
-- price 7.50 USD / 1M tokens.
--
-- Formula:
--   rate_multiplier = monthly_price_usd / (token_millions * 7.50)
--
-- Target table:
--   Basic Pass:     $14.68 / month ->   85M tokens -> 0.02302745x
--   Super Pass:     $36.71 / month ->  220M tokens -> 0.02224848x
--   Ultra Pass:     $73.42 / month ->  450M tokens -> 0.02175407x
--   God(神) Pass:   $146.84 / month -> 1000M tokens -> 0.01957867x
--
-- Plan 7 / Enterprise is no longer a monthly subscription pass; Enterprise is a
-- balance top-up tier handled by balance_packages, so hide/archive it here.

BEGIN;

WITH fixed_passes(slot, plan_name, price_usd, token_millions, sort_order) AS (
    VALUES
        (3, 'V-Claw Basic Pass', 14.68::numeric, 85::numeric, 10),
        (4, 'V-Claw Super Pass', 36.71::numeric, 220::numeric, 20),
        (5, 'V-Claw Ultra Pass', 73.42::numeric, 450::numeric, 30),
        (6, 'V-Claw God(神) Pass', 146.84::numeric, 1000::numeric, 40)
), updated_groups AS (
    UPDATE groups g
    SET name = fp.plan_name,
        description = fp.token_millions::text || 'M token-equivalent monthly subscription',
        rate_multiplier = ROUND(fp.price_usd / (fp.token_millions * 7.50), 8),
        daily_limit_usd = NULL,
        weekly_limit_usd = NULL,
        monthly_limit_usd = ROUND(fp.price_usd, 2),
        subscription_type = 'subscription',
        status = 'active',
        default_validity_days = 30,
        sort_order = fp.sort_order,
        token_price_per_million = 7.50,
        pricing_reference_model = 'gpt-5.5',
        input_output_ratio = 0.90,
        updated_at = NOW()
    FROM fixed_passes fp
    JOIN subscription_plans sp ON sp.id = fp.slot
    WHERE sp.group_id = g.id
    RETURNING sp.id AS plan_id, fp.plan_name, fp.price_usd, fp.token_millions, fp.sort_order
)
UPDATE subscription_plans sp
SET name = ug.plan_name,
    product_name = ug.plan_name,
    description = ug.token_millions::text || 'M token-equivalent monthly subscription',
    price = ROUND(ug.price_usd, 2),
    original_price = ROUND(ug.token_millions * 7.50, 2),
    validity_days = 30,
    validity_unit = 'day',
    for_sale = true,
    sort_order = ug.sort_order,
    features = concat_ws(E'\n',
        'pricing.version=ledger_v1',
        'pricing.formula=convert_ledger_amounts',
        'pricing.currency_source=selected_payment_currency',
        'pricing.token_price_ledger=7.50',
        'pricing.total_price_ledger=' || ROUND(ug.price_usd, 2)::text,
        'pricing.raw_value_ledger=' || ROUND(ug.token_millions * 7.50, 2)::text,
        'pricing.token_quantity_millions=' || ug.token_millions::text,
        'pricing.savings_formula=1 - total_price_ledger / raw_value_ledger'
    ),
    updated_at = NOW()
FROM updated_groups ug
WHERE sp.id = ug.plan_id;

UPDATE subscription_plans sp
SET for_sale = false,
    sort_order = 999,
    description = CASE
        WHEN COALESCE(sp.description, '') LIKE '%Archived after V-Claw monthly subscription pass correction.%'
            THEN sp.description
        WHEN trim(both E'\n' FROM COALESCE(sp.description, '')) = ''
            THEN 'Archived after V-Claw monthly subscription pass correction.'
        ELSE trim(both E'\n' FROM COALESCE(sp.description, '')) || E'\nArchived after V-Claw monthly subscription pass correction.'
    END,
    updated_at = NOW()
WHERE sp.id = 7;

UPDATE groups g
SET status = 'inactive',
    sort_order = 999,
    updated_at = NOW()
WHERE g.id = (SELECT sp.group_id FROM subscription_plans sp WHERE sp.id = 7);

COMMIT;
