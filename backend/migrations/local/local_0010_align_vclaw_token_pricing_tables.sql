-- Local correction: align V-Claw top-up balance packages and monthly subscription
-- pricing with the approved USD-first token table. CNY is derived later through
-- PAYMENT_MANUAL_FX_RATES_JSON.

-- Keep USD as source of truth; this FX rate makes the reference CNY outputs
-- in the approved table resolve from USD at checkout/display time.
UPDATE settings
SET value = jsonb_set(
    COALESCE(NULLIF(value, '')::jsonb, '{}'::jsonb),
    '{CNY}',
    '0.14684'::jsonb,
    true
)::text
WHERE key = 'PAYMENT_MANUAL_FX_RATES_JSON';

UPDATE settings
SET value = NOW() AT TIME ZONE 'UTC'
WHERE key = 'PAYMENT_FX_RATES_UPDATED_AT';

-- Balance/top-up packages: one-time tokens, permanent balance.
-- Store USD ledger price and raw GPT-5.5-equivalent USD allowance; actual_credits
-- is computed from amount_ledger * credit_multiplier / balance group rate.
-- Benchmark: $17.50 / 1M tokens => $0.175 per 10k credits.
WITH package_data(code, label, amount_usd, token_millions, badge, sort_order) AS (
    VALUES
        ('standard', 'Standard', 7.34::numeric, 27::numeric, 'Tiết kiệm 95.4%', 10),
        ('pro', 'Pro', 14.68::numeric, 63::numeric, 'Tiết kiệm 97.3%', 20),
        ('expert', 'Expert', 29.37::numeric, 130::numeric, 'Tiết kiệm 98.1%', 30),
        ('business', 'Business', 73.42::numeric, 340::numeric, 'Tiết kiệm 98.5%', 40),
        ('enterprise', 'Enterprise', 146.84::numeric, 700::numeric, 'Tiết kiệm 98.7%', 50)
), computed AS (
    SELECT
        code,
        label,
        amount_usd,
        token_millions,
        (token_millions * 17.50)::numeric AS credit_usd,
        ((token_millions * 17.50) / amount_usd)::numeric AS multiplier,
        badge,
        sort_order
    FROM package_data
)
UPDATE balance_packages bp
SET label = c.label,
    amount_ledger = ROUND(c.amount_usd, 2),
    credit_ledger = ROUND(c.credit_usd, 2),
    bonus_ledger = ROUND(c.credit_usd - c.amount_usd, 2),
    credit_multiplier = ROUND(c.multiplier, 6),
    actual_credits = (c.token_millions * 1000000)::bigint,
    credit_unit = 'tokens',
    badge = c.badge,
    sort_order = c.sort_order,
    for_sale = true
FROM computed c
WHERE bp.code = c.code;

-- Balance usage groups represent raw GPT-5.5-equivalent token burn, not subscription tiers.
UPDATE groups g
SET rate_multiplier = 0.175,
    daily_limit_usd = NULL,
    weekly_limit_usd = NULL,
    monthly_limit_usd = NULL,
    subscription_type = 'standard',
    status = 'active'
WHERE g.id IN (SELECT DISTINCT group_id FROM balance_packages WHERE group_id IS NOT NULL);

-- Monthly subscriptions: USD-first; CNY display is derived by FX.
WITH subscription_data(slot, name, price_usd, token_millions, sort_order) AS (
    VALUES
        (3, 'Basic Pass', 14.68::numeric, 85::numeric, 10),
        (4, 'Super Pass', 36.71::numeric, 220::numeric, 20),
        (5, 'Ultra Pass', 73.42::numeric, 450::numeric, 30),
        (6, 'God(神) Pass', 146.84::numeric, 1000::numeric, 40)
), computed_subscription AS (
    SELECT
        slot,
        name,
        price_usd,
        token_millions,
        (token_millions * 17.50)::numeric AS raw_value_usd,
        (price_usd / (token_millions * 17.50))::numeric AS rate_multiplier,
        sort_order
    FROM subscription_data
), updated_groups AS (
    UPDATE groups g
    SET name = 'V-Claw ' || cs.name,
        description = cs.token_millions::text || 'M token-equivalent monthly subscription',
        rate_multiplier = ROUND(cs.rate_multiplier, 8),
        daily_limit_usd = NULL,
        weekly_limit_usd = NULL,
        monthly_limit_usd = ROUND(cs.price_usd, 2),
        subscription_type = 'subscription',
        status = 'active',
        sort_order = cs.sort_order
    FROM computed_subscription cs, subscription_plans sp
    WHERE sp.id = cs.slot
      AND sp.group_id = g.id
    RETURNING sp.id AS plan_id, cs.*
)
UPDATE subscription_plans sp
SET name = ug.name,
    description = ug.token_millions::text || 'M token-equivalent monthly subscription',
    price = ROUND(ug.price_usd, 2),
    original_price = ROUND(ug.raw_value_usd, 2),
    validity_days = 30,
    for_sale = true,
    sort_order = ug.sort_order,
    features = concat_ws(E'\n',
        'pricing.version=ledger_v1',
        'pricing.formula=convert_ledger_amounts',
        'pricing.currency_source=selected_payment_currency',
        'pricing.token_price_ledger=17.50',
        'pricing.total_price_ledger=' || ROUND(ug.price_usd, 2)::text,
        'pricing.raw_value_ledger=' || ROUND(ug.raw_value_usd, 2)::text,
        'pricing.token_quantity_millions=' || ug.token_millions::text,
        'pricing.savings_formula=1 - total_price_ledger / raw_value_ledger'
    )
FROM updated_groups ug
WHERE sp.id = ug.plan_id;

-- Hide the old fifth subscription-table row; Enterprise is a top-up tier now, not
-- part of the four monthly subscription passes in the approved table.
UPDATE subscription_plans
SET for_sale = false,
    sort_order = 999,
    description = COALESCE(description, '') || E'\nArchived after V-Claw monthly subscription table correction.'
WHERE id = 7;

UPDATE groups
SET status = 'inactive',
    sort_order = 999
WHERE id = (SELECT group_id FROM subscription_plans WHERE id = 7);
