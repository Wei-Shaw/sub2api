# V-Claw entitlement mapping for sub2api

## Purpose

This document defines the backend contract V-Claw should consume after a user buys a subscription-backed package. sub2api owns the real entitlement primitives: subscription group, payment plan, API-key/group binding, usage/quota burn, refresh/switch APIs, balance fallback metadata, and checkout `plan.pricing` metadata. V-Claw's pricing UI must render `plan.pricing` as a formula/spec against the user's selected payment currency and must not expose raw `rate_multiplier` values to users.

## Current production snapshot checked

Production DB was inspected on the token.v-claw.org host. Credentials/connection details are intentionally not recorded here.

Current notable rows are **not** yet the target pricing table:

| group_id | name | platform | subscription_type | rate_multiplier | daily_limit_usd | weekly_limit_usd | monthly_limit_usd |
|---:|---|---|---|---:|---:|---:|---:|
| 8 | OpenAI-Subcription | openai | subscription | 0.1000 | 5 | 10 | 0 |

Implication: production currently has only one sellable-style subscription group discovered for OpenAI subscription usage, and it uses daily/weekly limits rather than the desired five-tier monthly quota table.

## Billing/rate model verified in code

The key formula in `backend/internal/service/billing_service.go` is:

```text
actual_subscription_usage_usd = raw_model_cost_usd * rate_multiplier
```

`actual_subscription_usage_usd` is what burns subscription quota (`daily_usage_usd`, `weekly_usage_usd`, `monthly_usage_usd`). Therefore:

- lower `rate_multiplier` = quota burns slower = user receives more raw model usage/tokens;
- `monthly_limit_usd` should be the ledger quota for that tier;
- `price` in `subscription_plans` is checkout price;
- `original_price` in `subscription_plans` can store the raw benchmark value used to display savings;
- CNY/VND/KRW are payment/display currencies only; USD remains the ledger/quota base.

## Pricing table calculation

The provided pricing screenshot implies a raw benchmark of about **17.50 USD / 1M tokens** (about **119.18 CNY / 1M tokens** at 6.81 CNY/USD). For each tier:

```text
raw_value_usd = target_token_millions * 17.50
rate_multiplier = monthly_limit_usd / raw_value_usd
monthly_limit_usd = checkout_price_usd
```

Recommended entitlement config matching the screenshot:

| Tier | Price CNY reference | Price USD / monthly_limit_usd | Target tokens | Raw value CNY reference | Raw value USD | rate_multiplier | Display saving |
|---|---:|---:|---:|---:|---:|---:|---:|
| Standard | 149 | 21.88 | 27M | 3,217 | 472.39 | 0.0463 | 95.4% |
| Pro | 199 | 29.22 | 63M | 7,508 | 1,102.50 | 0.0265 | 97.3% |
| Expert | 299 | 43.91 | 130M | 15,493 | 2,275.04 | 0.0193 | 98.1% |
| Business | 599 | 87.96 | 340M | 40,521 | 5,950.22 | 0.0148 | 98.5% |
| Enterprise | 1,099 | 161.38 | 700M | 83,426 | 12,250.51 | 0.0132 | 98.7% |

The CNY columns are **reference output**, not values to hardcode in `plan.pricing`. Store the formula inputs in ledger currency and let V-Claw convert to the selected payment currency using checkout FX metadata.

`plan.pricing` should look like this after backend parsing:

```json
{
  "version": "ledger_v1",
  "formula": "convert_ledger_amounts",
  "currency_source": "selected_payment_currency",
  "token_price_ledger": 17.5,
  "total_price_ledger": 21.88,
  "raw_value_ledger": 472.39,
  "token_quantity_millions": 27,
  "savings_formula": "1 - total_price_ledger / raw_value_ledger"
}
```

V-Claw renders:

```text
display_total = convert(total_price_ledger, ledger_currency -> selected_payment_currency)
display_token_price = convert(token_price_ledger, ledger_currency -> selected_payment_currency)
display_raw_value = convert(raw_value_ledger, ledger_currency -> selected_payment_currency)
display_saving_percent = (1 - total_price_ledger / raw_value_ledger) * 100
display_token_quantity = token_quantity_millions + localized million-token label
```

Do **not** keep `rate_multiplier = 1` for these subscription tiers. That would make `monthly_limit_usd` burn at raw model price and would not produce the marketed token-equivalent quota.

## Backend setup model

Use one `groups` row per effective economics tier because each tier has a different `rate_multiplier`.

Use one `subscription_plans` row per sellable package because plan rows control checkout/display and point to the entitlement group by `group_id`.

### Recommended group semantics

For each tier group:

- `platform = 'openai'` for the OpenAI-backed package table shown here;
- `subscription_type = 'subscription'`;
- `rate_multiplier` = tier multiplier from the table above;
- `monthly_limit_usd` = USD checkout price from the table above;
- `daily_limit_usd = NULL` and `weekly_limit_usd = NULL` initially, unless product explicitly wants daily/weekly throttles;
- `default_validity_days = 30`;
- `status = 'active'`;
- `supported_model_scopes` should match the route/model family V-Claw needs. For OpenAI groups this can be `['openai']` or the repo's existing OpenAI model-scope convention.

### Safe SQL template — review before production apply

This is an operator template, not yet applied by this document.

```sql
-- 1) Create/ensure one entitlement group per tier.
-- Adjust supported_model_scopes / routing fields to match existing OpenAI group conventions before running.
WITH tier_groups(name, description, rate_multiplier, monthly_limit_usd, sort_order) AS (
  VALUES
    ('V-Claw Standard',   'V-Claw 27M token-equivalent monthly package', 0.0463::numeric,  21.88::numeric, 10),
    ('V-Claw Pro',        'V-Claw 63M token-equivalent monthly package', 0.0265::numeric,  29.22::numeric, 20),
    ('V-Claw Expert',     'V-Claw 130M token-equivalent monthly package',0.0193::numeric,  43.91::numeric, 30),
    ('V-Claw Business',   'V-Claw 340M token-equivalent monthly package',0.0148::numeric,  87.96::numeric, 40),
    ('V-Claw Enterprise', 'V-Claw 700M token-equivalent monthly package',0.0132::numeric, 161.38::numeric, 50)
)
INSERT INTO groups (
  name,
  description,
  platform,
  subscription_type,
  rate_multiplier,
  daily_limit_usd,
  weekly_limit_usd,
  monthly_limit_usd,
  default_validity_days,
  status,
  sort_order,
  supported_model_scopes
)
SELECT
  name,
  description,
  'openai',
  'subscription',
  rate_multiplier,
  NULL,
  NULL,
  monthly_limit_usd,
  30,
  'active',
  sort_order,
  '["openai"]'::jsonb
FROM tier_groups
ON CONFLICT (name) DO UPDATE SET
  description = EXCLUDED.description,
  platform = EXCLUDED.platform,
  subscription_type = EXCLUDED.subscription_type,
  rate_multiplier = EXCLUDED.rate_multiplier,
  daily_limit_usd = EXCLUDED.daily_limit_usd,
  weekly_limit_usd = EXCLUDED.weekly_limit_usd,
  monthly_limit_usd = EXCLUDED.monthly_limit_usd,
  default_validity_days = EXCLUDED.default_validity_days,
  status = EXCLUDED.status,
  sort_order = EXCLUDED.sort_order,
  supported_model_scopes = EXCLUDED.supported_model_scopes,
  updated_at = NOW();

-- 2) Create/ensure one sellable plan per group.
WITH tier_plans(plan_name, product_name, group_name, price, original_price, token_millions, sort_order) AS (
  VALUES
    ('Standard',   'V-Claw Standard',   'V-Claw Standard',    21.88::numeric,   472.39::numeric,  27::numeric, 10),
    ('Pro',        'V-Claw Pro',        'V-Claw Pro',         29.22::numeric,  1102.50::numeric,  63::numeric, 20),
    ('Expert',     'V-Claw Expert',     'V-Claw Expert',      43.91::numeric,  2275.04::numeric, 130::numeric, 30),
    ('Business',   'V-Claw Business',   'V-Claw Business',    87.96::numeric,  5950.22::numeric, 340::numeric, 40),
    ('Enterprise', 'V-Claw Enterprise', 'V-Claw Enterprise', 161.38::numeric, 12250.51::numeric, 700::numeric, 50)
)
INSERT INTO subscription_plans (
  group_id,
  name,
  description,
  price,
  original_price,
  validity_days,
  validity_unit,
  features,
  product_name,
  for_sale,
  sort_order
)
SELECT
  g.id,
  p.plan_name,
  format('%s package; 30-day token-equivalent subscription quota.', p.product_name),
  p.price,
  p.original_price,
  30,
  'day',
  concat_ws(E'\n',
    'pricing.version=ledger_v1',
    'pricing.formula=convert_ledger_amounts',
    'pricing.currency_source=selected_payment_currency',
    'pricing.token_price_ledger=17.50',
    format('pricing.total_price_ledger=%s', p.price),
    format('pricing.raw_value_ledger=%s', p.original_price),
    format('pricing.token_quantity_millions=%s', p.token_millions),
    'pricing.savings_formula=1 - total_price_ledger / raw_value_ledger'
  ),
  p.product_name,
  TRUE,
  p.sort_order
FROM tier_plans p
JOIN groups g ON g.name = p.group_name
ON CONFLICT (name) DO UPDATE SET
  group_id = EXCLUDED.group_id,
  description = EXCLUDED.description,
  price = EXCLUDED.price,
  original_price = EXCLUDED.original_price,
  validity_days = EXCLUDED.validity_days,
  validity_unit = EXCLUDED.validity_unit,
  features = EXCLUDED.features,
  product_name = EXCLUDED.product_name,
  for_sale = EXCLUDED.for_sale,
  sort_order = EXCLUDED.sort_order,
  updated_at = NOW();
```

Before running, verify whether production has a partial unique index on `groups.name` because soft-delete migrations may make plain `ON CONFLICT (name)` invalid. If so, use explicit update-then-insert logic or the admin UI instead.

## Implemented backend endpoints

Authenticated user endpoints:

```http
GET  /api/v1/user/entitlements
POST /api/v1/user/entitlements/refresh
POST /api/v1/user/entitlements/switch
```

### GET /api/v1/user/entitlements

Returns the user's current API-key/group binding, owned subscription entitlements, and balance fallback state.

Important response fields:

```json
{
  "current": {
    "api_key_id": 100,
    "group_id": 8,
    "group_name": "V-Claw Standard",
    "group_platform": "openai",
    "mode": "subscription",
    "rate_multiplier": 0.0463,
    "supported_model_scopes": ["openai"],
    "monthly_limit_usd": 21.88,
    "monthly_usage_usd": 1.25
  },
  "api_key": {
    "id": 100,
    "group_id": 8,
    "status": "active"
  },
  "entitlements": [
    {
      "group_id": 8,
      "group_name": "V-Claw Standard",
      "group_platform": "openai",
      "mode": "subscription",
      "status": "active",
      "starts_at": "2026-05-12T00:00:00Z",
      "expires_at": "2026-06-11T00:00:00Z",
      "daily_usage_usd": 0,
      "weekly_usage_usd": 0,
      "monthly_usage_usd": 1.25,
      "monthly_limit_usd": 21.88,
      "rate_multiplier": 0.0463,
      "supported_model_scopes": ["openai"],
      "switchable": true,
      "current": true,
      "subscription_id": 7,
      "fallback_group_id": 2
    }
  ],
  "fallback": {
    "mode": "balance",
    "available": true,
    "balance_usd": 12.5
  }
}
```

### POST /api/v1/user/entitlements/refresh

Same response as `GET /api/v1/user/entitlements`. V-Claw should call this after checkout return / payment success polling so the desktop app can resync the server-side key/group binding.

### POST /api/v1/user/entitlements/switch

Request:

```json
{
  "group_id": 8,
  "api_key_id": 100
}
```

`api_key_id` is optional. If omitted, backend selects the user's active API key. The backend validates binding through the existing API-key update path, so subscription-only groups still require an active user subscription.

Response:

```json
{
  "api_key": {
    "id": 100,
    "group_id": 8,
    "status": "active"
  },
  "state": { "...": "same shape as entitlements response" }
}
```

## Payment fulfillment behavior

For subscription orders, sub2api now attempts non-blocking active binding after subscription assignment:

1. Fulfill subscription with `AssignOrExtendSubscription`.
2. Call entitlement binder with `user_id` + purchased `group_id`.
3. Binder switches the selected active API key to the purchased group using `APIKeyService.Update`, preserving existing group validation and cache invalidation.
4. If the user has no active API key or binding fails, payment fulfillment does **not** fail; backend writes a warning/audit event and V-Claw can still call refresh/switch later.

## V-Claw consumption flow

### After checkout success

1. V-Claw receives payment success / user returns from checkout.
2. V-Claw calls:

```http
POST /api/v1/user/entitlements/refresh
```

3. If `current.group_id` or `api_key.group_id` changed, V-Claw should persist the selected package and restart/reload the local gateway runtime.

### Manual package switch

1. Show `entitlements[]` where `switchable = true`.
2. User selects a package.
3. Call:

```http
POST /api/v1/user/entitlements/switch
{"group_id": 8}
```

4. Persist current package from response.
5. Restart/reload gateway runtime.

### Subscription exhausted / fallback

V-Claw should use `fallback.available` from entitlement state to decide UI copy:

- `fallback.available = true`: user can continue with balance/credit fallback.
- `fallback.available = false`: user needs recharge or subscription renewal.

If gateway later returns detailed quota/billing errors, V-Claw should map them into the same UX states:

- subscription quota exhausted + fallback available → continue using credit/balance;
- subscription quota exhausted + fallback unavailable → show recharge/upgrade CTA.

## What V-Claw should not do

- Do not calculate backend usage/quota by itself from the pricing table.
- Do not keep a single subscription group if tiers require different effective `rate_multiplier` values.
- Do not assume CNY is ledger currency.
- Do not bind arbitrary groups client-side; always call backend `switch` so subscription/group authorization and cache invalidation remain centralized.
