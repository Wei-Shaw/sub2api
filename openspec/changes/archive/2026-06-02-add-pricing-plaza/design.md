# Design: Pricing Plaza

## Context

The deployment already encodes everything needed to render a "compare prices and plans" page, but the data lives in five distinct places:

| Source | Holds | Lives in |
|---|---|---|
| LiteLLM JSON snapshot (≈185 KB) | Upstream USD per-token / per-image prices | `internal/service/litellm_pricing_service.go` |
| Hardcoded fallback table | Prices for models LiteLLM hasn't catalogued | `internal/service/model_pricing_resolver.go` |
| `Group.rate_multiplier` / `image_rate_multiplier` / `image_rate_independent` | Per-group nominal discount, optionally a separate image multiplier | `ent/schema/group.go` |
| `Group.image_price_1k/2k/4k` | Hard overrides for image tiers | `ent/schema/group.go` |
| `account.Credentials["model_mapping"]` keys | The set of models actually wired up under each account; account carries `GroupIDs []int64` | `internal/service/account.go` (`Account.GetModelMapping`) |
| `SubscriptionPlan.{price, original_price, features, validity_days, for_sale, sort_order}` | Full subscription product catalog | `ent/schema/subscription_plan.go` |
| `BALANCE_RECHARGE_MULTIPLIER` (system setting) | `1 CNY recharge → multiplier USD credit`, i.e. CNY→USD coefficient | `internal/service/payment_config_service.go` |

The plaza assembles these into two read-only views without touching the billing path.

## Goals / Non-Goals

**Goals**

- Anonymous, public, cache-friendly endpoints that answer "what models, at what price, with what discount."
- Reuse all existing pricing primitives—zero new tables, zero new system settings, zero changes to billing or payment collection.
- Dual-currency display (CNY default, USD switch) using only the already-configured `BalanceRechargeMultiplier`.
- Tolerate LiteLLM gaps via the existing fallback table without surfacing "missing data" UI states.

**Non-Goals**

- No reflection of channel-level price overrides or model-name remapping. The plaza is an honest reflection of *list* price, not realized billing price. The deployer accepts this.
- No tier-interval pricing display, no cache-write/cache-read columns, no per-request column. Tokens and images only.
- No "buy" CTA, no auth-aware "your discount" highlight—plaza is purely informational in v1.
- No new admin field for plaza visibility, sort order, or feature lists—we ride existing fields (`status`, `is_exclusive`, `id` order, `sort_order` on plans).

## Key Decisions

### Decision 1: Plaza is decoupled from `Channel`

**What**: Model rows are assembled from `Group → Account → keys(account.Credentials["model_mapping"])`, never `Channel.*`. Channel-level price overrides (`channel_model_pricing`), channel-level model-name mappings, and channel restrictions are all ignored by the plaza.

**Why**: The user explicitly chose this so each (group, model) pair has a *single* deterministic price = `base × group.rate_multiplier`. This makes the discount column unambiguous (uniform across input/output), and avoids combinatorial explosion when a model is reachable through multiple channels.

**Trade-off**: If a deployment heavily uses `channel_model_pricing` to override list price for some models, plaza display will diverge from realized billing for those models. The user has chosen not to surface a disclaimer; this is treated as a known and accepted divergence.

**Alternatives rejected**: Showing realized per-channel price (rejected: makes the table 5–10× wider and reintroduces the "no single price" problem).

### Decision 2: Model set comes from `account.Credentials["model_mapping"]`, not from LiteLLM model catalog

**What**: For each public group, model set = ⋃ `keys(account.Credentials["model_mapping"])` over every active account `A` such that `G.id ∈ A.GroupIDs`. Wildcard keys (e.g. `claude-3-*`) are stripped — the plaza only lists concrete model names. The displayed `platform` for each model is `account.Platform`.

**Why**: This guarantees we only show models that are *actually wired up* in this deployment. Listing every model LiteLLM knows about would surface dozens of historical / unsupported models per platform, and listing every fallback-table entry would surface internal SKUs. Accounts are the deployment's primary registry of `(group, model)` association — each account explicitly declares the request-side model names it accepts via `Credentials["model_mapping"]`.

**Detail**: A group with zero active accounts (or only accounts whose `model_mapping` is empty / contains only wildcards) yields zero rows—naturally filtered out. Channels are explicitly **not** consulted: their `model_pricing` / `model_mapping` and `restrict_models` lists are all ignored, see Decision 3 and the "Channel Configuration Decoupling" requirement in the spec.

### Decision 3: Price resolution chain, with `getImageUnitPrice` reuse

For each `(group, model)`:

```
1. Look up LiteLLM pricing for `model`.
2. If LiteLLM miss → look up hardcoded fallback table.
3. If fallback also miss → drop the row (do not render).

If LiteLLM hit and `mode == "image_generation"` → image branch.
Otherwise (token-style billing) → token branch.
```

**Token branch**:
```
base.input_per_token  = pricing.input_cost_per_token   (USD)
base.output_per_token = pricing.output_cost_per_token  (USD)
site.input_per_token  = base.input_per_token  × group.rate_multiplier
site.output_per_token = base.output_per_token × group.rate_multiplier
multiplier_displayed  = group.rate_multiplier
discount_percent      = (1 - rate_multiplier) × 100      // -inf possible if >1
```

**Image branch**: reuse `getImageUnitPrice` from `billing_service.go` (already implements the full "group override → LiteLLM × tier-multiplier × image_rate_multiplier" chain) for each of `1K`, `2K`, `4K`. The plaza renders these as **three columns on a single row** (per the user's UI choice). The displayed multiplier on this row is `image_rate_independent ? image_rate_multiplier : rate_multiplier`. Discount per tier is computed against the upstream tier base price (`base.image × tier_factor`).

### Decision 4: Dual-currency via existing `BalanceRechargeMultiplier`

`BalanceRechargeMultiplier` semantically encodes `1 CNY recharge → multiplier USD credit` (verified at `payment_amounts.go:19` and `payment_order.go:60`).

```
USD per CNY = multiplier
CNY per USD = 1 / multiplier
```

The plaza endpoints return:

- **Model rows**: prices in **USD** (the natural unit of LiteLLM and the fallback table) plus the multiplier. To switch to CNY, frontend divides by `multiplier`. **Default display is CNY**.
- **Plan cards**: `price` / `original_price` returned **as stored** (CNY—matches current support of `payment_order.go:57` which directly bills `plan.Price` as CNY) plus the multiplier. To switch to USD, frontend multiplies by `multiplier`. **Default display is CNY**.

Both endpoints return the same `currency_meta` block:

```json
"currency_meta": {
  "balance_recharge_multiplier": 0.14,
  "model_native":  "USD",
  "plan_native":   "CNY"
}
```

The frontend currency toggle is a pure display layer—no refetching, no server-side currency parameter. This matches the user's directive to "default both views to CNY" while preserving lossless conversion.

### Decision 5: Group eligibility and ordering

A group is shown in the **model plaza** iff:

- `status == active`, AND
- `is_exclusive == false`, AND
- `subscription_type == standard` (subscription-typed groups belong on the plan plaza), AND
- has at least one row of model_mapping after dedup (across associated active accounts).

A plan is shown in the **plan plaza** iff:

- `for_sale == true`, AND
- the linked `Group` exists.

**Ordering**: groups by `id ASC`; within a group, models alphabetically; image rows after token rows when both apply for the same group. Plans by `sort_order ASC, id ASC` (existing `SubscriptionPlan` convention).

The user explicitly chose `id ASC` for groups—no new sort field added.

### Decision 6: Plan card "included models" comes from associated accounts

For each `SubscriptionPlan p`, the included-models list = ⋃ `keys(account.Credentials["model_mapping"])` for the active accounts associated with the `Group` referenced by `p.group_id` (the same union used by the model plaza). This requires no schema change. Limit display to the first ~10 names with a "+N more" affordance in the UI.

### Decision 7: Caching

In-memory cache keyed by `(group set hash, account model_mapping hash, system multiplier, plan revision)` with a 60-second TTL, refreshed on demand. The aggregation walks every active group; on a deployment with hundreds of groups this is non-trivial work that should not run on every request. The cache is invalidated when any of the underlying tables mutate (best-effort via existing repository hooks; if hooks are absent, the 60-second TTL is acceptable).

## Data Flow

```
                     ┌──────────────────────────────────────┐
                     │ GET /api/v1/plaza/models             │
                     │ (anonymous; query: group_id?,        │
                     │  platform?, q?)                       │
                     └─────────────────┬────────────────────┘
                                       │
                                       ▼
                  PlazaService.ListModelRows(filter)
                                       │
   ┌───────────────────────────────────┼─────────────────────────────────┐
   │ 1. groupRepo.ListPlazaEligible()                                    │
   │    where status=active, is_exclusive=false,                         │
   │          subscription_type=standard, ORDER BY id                    │
   │ 2. accountRepo.ListActive() → keep status=active, build              │
   │    map[group_id] -> []*Account                                       │
   │ 3. for each group:                                                   │
   │      models := union of keys(account.Credentials["model_mapping"])   │
   │                across its associated active accounts                 │
   │                (wildcard keys stripped)                              │
   │      for each model:                                                 │
   │         pricing := litellmSvc.Lookup(model)                          │
   │         if !pricing { pricing = fallbackTable[model] }              │
   │         if !pricing { skip }                                         │
   │         if pricing.mode == "image_generation":                       │
   │            row = imageRow(group, model, pricing) // 3 tiers         │
   │         else:                                                        │
   │            row = tokenRow(group, model, pricing)                    │
   │ 4. apply filter (group_id, platform, q)                             │
   │ 5. attach currency_meta = { multiplier, "USD", "CNY" }              │
   └─────────────────────────────────────────────────────────────────────┘
                                       │
                                       ▼
                              JSON response (USD numerics)


                     ┌──────────────────────────────────────┐
                     │ GET /api/v1/plaza/plans              │
                     │ (anonymous; no filters in v1)        │
                     └─────────────────┬────────────────────┘
                                       │
                                       ▼
                  PlazaService.ListPlanCards()
                                       │
   ┌───────────────────────────────────┼─────────────────────────────────┐
   │ 1. planRepo.ListForSale() ORDER BY sort_order, id                   │
   │ 2. for each plan:                                                    │
   │      group := groupRepo.GetByID(plan.group_id)                       │
   │      models := union of keys(account.Credentials["model_mapping"])   │
   │                across active accounts associated with group          │
   │      card = { plan, group_summary, models }                         │
   │ 3. attach currency_meta                                              │
   └─────────────────────────────────────────────────────────────────────┘
                                       │
                                       ▼
                          JSON response (CNY numerics)
```

## Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Plaza price diverges from realized billing for models with `channel_model_pricing` overrides | Accepted; plaza is "list price" by design. Documented in proposal. |
| LiteLLM JSON updates lag upstream price changes; plaza shows stale "base" | The 60-second cache plus periodic LiteLLM refresh (already wired) keep drift bounded; we do not surface a "data freshness" UI in v1. |
| Group with hundreds of models causes large response | Filters + per-group pagination not needed in v1; a single deployment is unlikely to have >2000 (group × model) rows. Cache absorbs cost. |
| Multiplier changes mid-session—frontend cached value gets stale | Toggle is a pure display transform; staleness shows as slightly off conversions, not incorrect billing. Acceptable. |
| Anonymous endpoints may be scraped | Rate-limited at the same edge layer that protects existing public settings endpoints (`/api/v1/settings/public`); no per-deployment unique data is leaked beyond what would already be visible to a paying user. |

## Migration Plan

None. No schema change, no data migration, no setting migration. Feature is purely additive at the API surface.

## Open Questions

None at this stage. All material decisions (UI form, anonymous access, multiplier semantics, model source, exclusion of channel pricing, image tier display, fallback policy, ordering, currency conventions) have been resolved with the user.
