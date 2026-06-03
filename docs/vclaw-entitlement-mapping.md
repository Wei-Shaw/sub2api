# V-Claw entitlement mapping for sub2api

## Purpose

This document defines the backend contract V-Claw should consume after a user buys a subscription-backed package. sub2api owns the real entitlement primitives: subscription group, payment plan, API-key/group binding, usage/quota burn, refresh/switch APIs, balance fallback metadata, and checkout `plan.pricing` metadata. V-Claw's pricing UI must render `plan.pricing` as a formula/spec against the user's selected payment currency and must not expose raw `rate_multiplier` values to users.

## Subscription pricing source of truth

`local/local_0018_recalculate_vclaw_subscription_passes.sql` is the repo-side correction for V-Claw monthly subscription pass economics. It repairs stale rows that were previously seeded/calculated from the wrong benchmark and hides the old Enterprise subscription plan.

Current subscription pass target:

| Plan | Monthly price / monthly_limit_usd | Target token-equivalent quota | Raw value ledger | rate_multiplier |
|---|---:|---:|---:|---:|
| V-Claw Basic Pass | $14.68 | 85M | $637.50 | 0.02302745 |
| V-Claw Super Pass | $36.71 | 220M | $1,650.00 | 0.02224848 |
| V-Claw Ultra Pass | $73.42 | 450M | $3,375.00 | 0.02175407 |
| V-Claw God(神) Pass | $146.84 | 1000M | $7,500.00 | 0.01957867 |

`V-Claw Enterprise` is not a monthly subscription pass in this table. It belongs to balance top-up packages and must be hidden/archived from `subscription_plans`.

## Billing/rate model verified in code

The key formula in `backend/internal/service/billing_service.go` is:

```text
actual_subscription_usage_usd = raw_model_cost_usd * rate_multiplier
```

`actual_subscription_usage_usd` is what burns subscription quota (`daily_usage_usd`, `weekly_usage_usd`, `monthly_usage_usd`). Therefore:

- lower `rate_multiplier` = quota burns slower = user receives more raw model usage/tokens;
- `monthly_limit_usd` should be the plan checkout price / monthly quota ledger;
- `price` in `subscription_plans` is checkout price;
- `original_price` stores the raw benchmark value for savings display;
- payment currencies are display/payment currencies only; USD remains the ledger/quota base.

## Pricing table calculation

Subscription pass multiplier is derived from the advertised token-equivalent quota and the GPT-5.5 weighted token price baseline:

```text
token_price_per_million = 7.50
raw_value_usd = target_token_millions * token_price_per_million
rate_multiplier = monthly_price_usd / raw_value_usd
monthly_limit_usd = monthly_price_usd
```

Example `plan.pricing` metadata for Basic Pass:

```json
{
  "version": "ledger_v1",
  "formula": "convert_ledger_amounts",
  "currency_source": "selected_payment_currency",
  "token_price_ledger": 7.5,
  "total_price_ledger": 14.68,
  "raw_value_ledger": 637.5,
  "token_quantity_millions": 85,
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

Do **not** use the old `17.50` token-price benchmark for these subscription passes. Do **not** keep the stale Enterprise subscription row sellable.

## Backend setup model

Use one `groups` row per effective economics tier because each tier has a different `rate_multiplier`.

Use one `subscription_plans` row per sellable package because plan rows control checkout/display and point to the entitlement group by `group_id`.

### Recommended group semantics

For each subscription pass group:

- `platform = 'openai'` for the OpenAI-backed package table shown here;
- `subscription_type = 'subscription'`;
- `rate_multiplier` = tier multiplier from the table above;
- `monthly_limit_usd` = USD checkout price from the table above;
- `daily_limit_usd = NULL` and `weekly_limit_usd = NULL` unless product explicitly wants daily/weekly throttles;
- `default_validity_days = 30`;
- `status = 'active'` for the 4 pass groups;
- `token_price_per_million = 7.50`;
- `pricing_reference_model = 'gpt-5.5'`;
- `input_output_ratio = 0.90`.

### Implemented local migration

`backend/migrations/local/local_0018_recalculate_vclaw_subscription_passes.sql` performs the repair idempotently:

1. updates plan IDs `3..6` and their linked groups to Basic/Super/Ultra/God pass values;
2. sets `rate_multiplier = ROUND(price_usd / (token_millions * 7.50), 8)`;
3. sets `monthly_limit_usd = price_usd`, clears daily/weekly limits;
4. rewrites `features` pricing metadata with `pricing.token_price_ledger=7.50`;
5. sets plan ID `7` / Enterprise `for_sale=false` and linked group `status='inactive'`.

Before any production config mutation, inspect current live rows and apply the migration/update only after explicit approval and target predicate confirmation.

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
    "group_name": "V-Claw Basic Pass",
    "group_platform": "openai",
    "mode": "subscription",
    "rate_multiplier": 0.02302745,
    "supported_model_scopes": ["openai"],
    "monthly_limit_usd": 14.68,
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
      "group_name": "V-Claw Basic Pass",
      "group_platform": "openai",
      "mode": "subscription",
      "status": "active",
      "starts_at": "2026-05-12T00:00:00Z",
      "expires_at": "2026-06-11T00:00:00Z",
      "daily_usage_usd": 0,
      "weekly_usage_usd": 0,
      "monthly_usage_usd": 1.25,
      "monthly_limit_usd": 14.68,
      "rate_multiplier": 0.02302745,
      "supported_model_scopes": ["openai"],
      "switchable": true,
      "current": true,
      "subscription_id": 101,
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
