# Add Pricing Plaza (Models + Subscription Plans)

## Why

Prospective users currently have **no public-facing way to compare what models a deployment offers, what they cost, and what discount each group provides** before signing up. The data is all there—LiteLLM upstream prices, `Group.rate_multiplier`, image-tier prices, `SubscriptionPlan` records—but it is locked behind admin pages and runtime billing code.

The existing `BALANCE_RECHARGE_MULTIPLIER` payment setting already encodes the CNY→USD conversion (1 CNY of recharge → `multiplier` USD of credit), so a dual-currency display does not require new infrastructure.

We need a public landing surface—the **Pricing Plaza**—so anonymous visitors can answer two questions at a glance:

1. *"What models can I use, what is the upstream list price, what is this site's price, and what is the effective discount?"*
2. *"What subscription plans are sold here, what do they include, and what is the savings vs. the original price?"*

## What Changes

Introduce a single new public capability `pricing-plaza` exposing two anonymous endpoints and two frontend views:

- **Model Plaza** (`GET /api/v1/plaza/models`): a flat list of `(group, model, pricing)` rows aggregated from `account.models` ∪ across non-exclusive active groups. Token models render one row per (group, model); image-generation models render one row with three price columns (1K / 2K / 4K). Each row includes upstream base price, site price, multiplier, and discount percent—all in **USD numerics**, with the current `BalanceRechargeMultiplier` returned alongside so the frontend can switch display between CNY (default) and USD without a refetch.

- **Subscription Plan Plaza** (`GET /api/v1/plaza/plans`): cards built from `SubscriptionPlan` rows where `for_sale = true`, joined with their associated `Group` and the union of `account.models` under that group. `price` and `original_price` are returned as stored (treated as CNY) and the frontend switches to USD by multiplying by `BalanceRechargeMultiplier`.

- **Frontend `/plaza` route(s)**: a tabbed page with the model table (currency toggle, group / platform / model-name filters) and the plan card grid. Both default to CNY display.

No changes to billing, payment collection, schemas, or admin behavior. The plaza is a **read-only display layer** sourcing already-resolved data.

## Impact

- **Affected specs**: NEW `pricing-plaza` capability.
- **Affected code (backend)**: new package `internal/service/pricing_plaza` that composes `LiteLLMPricingService` (with hardcoded fallback), `Group`, `Account`, and `SubscriptionPlan`; reuses `getImageUnitPrice` for image price resolution; new public route group under `internal/server/routes`. No modifications to existing pricing or billing code.
- **Affected code (frontend)**: new pages `/plaza/models` and `/plaza/plans` plus shared currency-toggle hook reading the multiplier from the response payload.
- **Anonymous access surface**: two new public GET endpoints (no auth, rate-limited at the edge along with other public settings endpoints).
- **Out of scope for this change**:
  - Channel-level price overrides (`channel_model_pricing`) and `channel.model_mapping` are deliberately ignored—plaza shows list price, not the calibrated billing price.
  - Per-request and tier-interval billing modes.
  - "Buy now" / "Create API key" CTAs—plaza is informational only in v1.
  - Persisting per-user historical multiplier values; the plaza always reflects the current `BalanceRechargeMultiplier`.
