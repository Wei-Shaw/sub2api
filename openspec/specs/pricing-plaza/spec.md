# Pricing Plaza

## Purpose

Provide a public, anonymous-accessible landing surface ("Pricing Plaza") so prospective users can compare offered models, their upstream and site prices, group discounts, and subscription plan offerings before signing up. The plaza is a read-only display layer sourced from already-resolved pricing data (LiteLLM upstream + hardcoded fallback, group rate multipliers, image tier overrides, and `SubscriptionPlan` records). It exposes two endpoints — `GET /api/v1/plaza/models` and `GET /api/v1/plaza/plans` — and reuses `BALANCE_RECHARGE_MULTIPLIER` for client-side CNY/USD display conversion. Channel-level overrides and per-request billing modes are deliberately excluded.
## Requirements
### Requirement: Anonymous Model Plaza Endpoint

The system SHALL expose `GET /api/v1/plaza/models` as an unauthenticated public endpoint that returns the list of `(group, model, pricing)` rows aggregated across all eligible groups, plus a `currency_meta` block carrying the current `BALANCE_RECHARGE_MULTIPLIER`. The endpoint SHALL NOT require any authentication header and SHALL ignore any credentials supplied in the request.

#### Scenario: Anonymous client requests model rows

- **WHEN** an anonymous client (no `Authorization` header, no API key) issues `GET /api/v1/plaza/models`
- **THEN** the response is `200 OK` with a JSON body of shape `{ "rows": [...], "currency_meta": { "balance_recharge_multiplier": <float>, "model_native": "USD", "plan_native": "CNY" } }`

#### Scenario: Invalid auth header is ignored

- **WHEN** a client issues `GET /api/v1/plaza/models` with `Authorization: Bearer not-a-real-token`
- **THEN** the response is `200 OK` and the response body is identical to what an anonymous client would receive

### Requirement: Anonymous Subscription Plan Plaza Endpoint

The system SHALL expose `GET /api/v1/plaza/plans` as an unauthenticated public endpoint that returns subscription plan cards plus the same `currency_meta` block. The endpoint SHALL NOT require authentication.

#### Scenario: Anonymous client requests plan cards

- **WHEN** an anonymous client issues `GET /api/v1/plaza/plans`
- **THEN** the response is `200 OK` with a JSON body of shape `{ "cards": [...], "currency_meta": { "balance_recharge_multiplier": <float>, "model_native": "USD", "plan_native": "CNY" } }`

### Requirement: Group Eligibility for Model Plaza

The system SHALL include a group on the model plaza if and only if **all** of the following hold: `status == active`, `is_exclusive == false`, `subscription_type == standard`, and the group has at least one model after computing the union of `account.Credentials["model_mapping"]` keys over the active accounts associated with that group. Groups failing any predicate SHALL produce zero rows.

#### Scenario: Exclusive group is hidden

- **WHEN** the system aggregates plaza rows and group `G` has `is_exclusive == true`
- **THEN** no rows for `G` appear in the response, regardless of `G`'s account or model configuration

#### Scenario: Subscription-typed group is hidden from model plaza

- **WHEN** the system aggregates plaza rows and group `G` has `subscription_type == "subscription"`
- **THEN** no rows for `G` appear in the model plaza response (such groups belong to the plan plaza)

#### Scenario: Inactive group is hidden

- **WHEN** the system aggregates plaza rows and group `G` has `status != "active"`
- **THEN** no rows for `G` appear

#### Scenario: Group with no accounts produces zero rows

- **WHEN** group `G` is otherwise eligible but has zero active accounts (or all associated accounts have an empty `model_mapping` entry)
- **THEN** `G` contributes zero rows to the response

### Requirement: Model Set From Account Model Mapping

For each eligible group, the system SHALL derive the model set as the deduplicated union of `keys(account.Credentials["model_mapping"])` (wildcard keys excluded) for every account `A` such that `A.status == active` AND `G.id ∈ A.GroupIDs`. The displayed `platform` for each model SHALL be `account.Platform`. The system SHALL NOT include models from `LiteLLM`, hardcoded fallback, or channels that are not present in any associated account's `model_mapping`.

#### Scenario: Two accounts with overlapping models

- **WHEN** group `G` is associated with accounts `A1` (`model_mapping = {"m1":"m1","m2":"m2"}`) and `A2` (`model_mapping = {"m2":"m2","m3":"m3"}`)
- **THEN** the system emits exactly three model rows for `G`: one each for `m1`, `m2`, `m3`

#### Scenario: Disabled account ignored

- **WHEN** group `G` is associated with accounts `A1` (`status=active`, mapping has `m1`) and `A2` (`status=disabled`, mapping has `m2`)
- **THEN** the response contains only the row for `m1`

#### Scenario: Wildcard mapping keys are skipped

- **WHEN** account `A` has `model_mapping = {"claude-*":"claude-3-5-sonnet","claude-opus-4-5":"claude-opus-4-5"}`
- **THEN** the plaza emits only the row for `claude-opus-4-5`; the wildcard pattern `"claude-*"` is ignored

#### Scenario: Account without model_mapping contributes nothing

- **WHEN** account `A` has no `model_mapping` entry in its `Credentials` (or it is empty) and is not on a platform with a default mapping (e.g. Antigravity)
- **THEN** `A` contributes zero models to its associated groups

### Requirement: Price Resolution Chain With Fallback

For each `(group, model)` pair, the system SHALL resolve a base price by: (1) querying the LiteLLM pricing service; (2) if missed, querying the hardcoded fallback table used by the billing service; (3) if both miss, the row SHALL be silently dropped from the response (no error, no placeholder row). The site price SHALL always equal `base × group.rate_multiplier` for token-billed models.

#### Scenario: LiteLLM hit

- **WHEN** LiteLLM returns pricing for `model = "claude-opus-4-5"` and group `G` has `rate_multiplier = 0.8`
- **THEN** the row's `base.input_per_token` and `base.output_per_token` come from LiteLLM, and `site.input_per_token = base.input_per_token × 0.8`, `site.output_per_token = base.output_per_token × 0.8`

#### Scenario: LiteLLM miss, fallback hit

- **WHEN** LiteLLM has no entry for `model = "private-llm-1"` but the hardcoded fallback table has it
- **THEN** the row is rendered using the fallback prices as `base`, with `site = base × group.rate_multiplier`

#### Scenario: Both miss

- **WHEN** neither LiteLLM nor the fallback table contains pricing for `model = "experimental-x"`
- **THEN** no row is emitted for `(any group, "experimental-x")` and the API response does not include a placeholder

### Requirement: Token Model Row Shape

For a model whose LiteLLM/fallback `mode` is anything other than `image_generation` (i.e. token-billed), the system SHALL emit one row per `(group, model)` containing token-prefix price fields and SHALL omit the `image_prices` block. The row SHALL carry **input**, **output**, **cache-write** and **cache-read** prices, each as both a `base` (upstream) and `site` (group-rate-adjusted) variant. Cache prices SHALL be sourced from the resolved `ModelPricing.CacheCreation5mPrice` and `ModelPricing.CacheReadPricePerToken` (single tier; the 1-hour cache tier is intentionally not surfaced in v1). When a cache price for the model is unknown or zero AND the model does not declare `SupportsCacheBreakdown`, the corresponding cache field SHALL be omitted from the JSON (`omitempty`) so clients can render `—` instead of `$0`.

#### Scenario: Token row fields present

- **WHEN** the response contains a row for a token model
- **THEN** the row contains `billing_type = "token"`, numeric `base.input_per_token`, `base.output_per_token`, `site.input_per_token`, `site.output_per_token`, `multiplier`, `discount_percent`, and the `image_prices` field is absent or `null`

#### Scenario: Discount calculation

- **WHEN** `group.rate_multiplier = 0.8` for a token row
- **THEN** the row's `discount_percent` equals `20.0` (computed as `(1 - rate_multiplier) × 100`, rounded to one decimal)

#### Scenario: Cache-bearing model exposes write/read prices

- **WHEN** the resolved `ModelPricing` for `claude-opus-4-5` reports `CacheCreation5mPrice = 3.75e-6` and `CacheReadPricePerToken = 0.3e-6`, and group `G` has `rate_multiplier = 0.8`
- **THEN** the row carries `cache_write_price_per_mtok ≈ 3.75`, `cache_read_price_per_mtok ≈ 0.30`, `site_cache_write_price_per_mtok ≈ 3.00`, `site_cache_read_price_per_mtok ≈ 0.24`

#### Scenario: Model without cache pricing omits cache fields

- **WHEN** the resolved `ModelPricing` for a model returns `0` for both `CacheCreation5mPrice` and `CacheReadPricePerToken` and `SupportsCacheBreakdown == false`
- **THEN** the row JSON does NOT include `cache_write_price_per_mtok`, `cache_read_price_per_mtok`, `site_cache_write_price_per_mtok`, or `site_cache_read_price_per_mtok` fields

#### Scenario: Model with explicit zero cache price still omits the field

- **WHEN** a model has `CacheCreation5mPrice == 0` because the upstream provider does not bill for cache writes (and `SupportsCacheBreakdown == false`)
- **THEN** the row omits `cache_write_price_per_mtok` rather than emitting `0`, so clients render `—`

### Requirement: Image Model Row Shape (Three Tier Columns)

For a model whose LiteLLM `mode == "image_generation"`, the system SHALL emit one row per `(group, model)` carrying three image price tiers (`tier_1k`, `tier_2k`, `tier_4k`) inside a single `image_prices` block. The system SHALL reuse the existing `getImageUnitPrice` function in `billing_service.go` to compute each tier so plaza prices match billing-side resolution. The displayed `multiplier` on the row SHALL equal `image_rate_multiplier` if `image_rate_independent == true`, otherwise `rate_multiplier`.

#### Scenario: Group with image price overrides

- **WHEN** group `G` has `image_price_1k = 0.032`, `image_price_2k = 0.048`, `image_price_4k = 0.064` and `image_rate_independent = false`
- **THEN** the image row's `image_prices.tier_1k.site = 0.032`, `tier_2k.site = 0.048`, `tier_4k.site = 0.064`, and the upstream `base` prices for each tier come from LiteLLM × the existing tier factors (`1.0×`, `1.5×`, `2.0×`)

#### Scenario: Group without image price overrides

- **WHEN** group `G` has all three `image_price_*` fields unset and `image_rate_independent = false`, `rate_multiplier = 0.8`
- **THEN** each tier's `site` equals LiteLLM-`base × tier_factor × 0.8`

#### Scenario: Independent image multiplier

- **WHEN** group `G` has `image_rate_independent = true` and `image_rate_multiplier = 0.5`, `rate_multiplier = 1.0`
- **THEN** the image row's displayed `multiplier = 0.5` and tier site prices use `0.5` rather than `1.0`

#### Scenario: Image and token rows coexist for same group

- **WHEN** group `G` has both token-billed and image-generation models in its account union
- **THEN** the response contains both kinds of rows for `G`; token rows precede image rows in the per-group ordering

### Requirement: Currency Conventions and Toggle Source

The system SHALL return model row prices as numeric values in **USD** (the natural unit of LiteLLM and the fallback table) and subscription plan prices as numeric values in **CNY** (the unit at which `SubscriptionPlan.price` is collected by the payment layer). Both endpoints SHALL include a `currency_meta.balance_recharge_multiplier` field reading the live `BALANCE_RECHARGE_MULTIPLIER` system setting so clients can convert losslessly between display currencies. The endpoints SHALL NOT accept a `currency` query parameter; conversion is a client-side display concern.

#### Scenario: Multiplier reflects current setting

- **WHEN** `BALANCE_RECHARGE_MULTIPLIER` is set to `0.14` at the moment of the request
- **THEN** both endpoints return `currency_meta.balance_recharge_multiplier == 0.14`

#### Scenario: Native units declared

- **WHEN** any plaza endpoint is called
- **THEN** the response declares `currency_meta.model_native == "USD"` and `currency_meta.plan_native == "CNY"`

#### Scenario: No server-side currency switch

- **WHEN** a client passes `?currency=CNY` to either plaza endpoint
- **THEN** the server ignores the parameter and returns prices in the natively declared units

### Requirement: Model Plaza Filter Parameters

`GET /api/v1/plaza/models` SHALL accept three optional query parameters: `group_id` (exact match against `group.id`), `platform` (exact match against `group.platform`), and `q` (case-insensitive substring match against the model name). The `q` parameter SHALL be rejected with HTTP `400` if longer than 64 characters. Filters apply after row aggregation; an empty result is a valid `200` response.

#### Scenario: Group filter narrows result

- **WHEN** the client requests `/api/v1/plaza/models?group_id=12`
- **THEN** the response contains rows only for the group whose `id == 12`

#### Scenario: Search filter is case-insensitive substring

- **WHEN** the client requests `/api/v1/plaza/models?q=opus`
- **THEN** the response contains every row whose model name (case-folded) contains the substring `"opus"`

#### Scenario: Empty filter result

- **WHEN** the client requests a filter combination that matches no rows
- **THEN** the response is `200 OK` with `rows: []` and `currency_meta` populated

#### Scenario: Oversized search string rejected

- **WHEN** the client requests `/api/v1/plaza/models?q=<65-or-more-char-string>`
- **THEN** the response is `400` with an error code indicating the search parameter is too long

### Requirement: Subscription Plan Card Shape

Each card in the plan plaza response SHALL include `id`, `name`, `price`, `original_price`, `discount_percent` (computed as `(1 - price/original_price) × 100` when `original_price > price`, otherwise `0`), `features`, `validity_days`, `group_summary` (summarizing the linked group), and `models` (the union of `keys(account.Credentials["model_mapping"])` over active accounts associated with the linked group, wildcards excluded, capped at the first 50 names with the remainder count exposed as `models_overflow`).

#### Scenario: Plan with original price greater than price

- **WHEN** a `SubscriptionPlan` has `price = 199`, `original_price = 299`
- **THEN** the card's `discount_percent ≈ 33.4` (rounded to one decimal)

#### Scenario: Plan with original price equal to price

- **WHEN** a plan has `price == original_price`
- **THEN** the card's `discount_percent == 0` and the frontend is expected to omit the struck-through original-price line

#### Scenario: Plan models cap

- **WHEN** the linked group's union of account `model_mapping` keys has 75 distinct names
- **THEN** the card's `models` array contains the first 50 names and `models_overflow == 25`

### Requirement: Plan Eligibility for Plaza

The system SHALL include a subscription plan on the plan plaza if and only if `for_sale == true` AND its linked `Group` exists with `status == active`. Plans with missing or inactive groups SHALL be excluded.

#### Scenario: For-sale plan with active group

- **WHEN** plan `P` has `for_sale = true` and its linked group has `status = active`
- **THEN** `P` appears in the plaza response

#### Scenario: For-sale plan with inactive group

- **WHEN** plan `P` has `for_sale = true` but its linked group has `status = "disabled"`
- **THEN** `P` does NOT appear in the plaza response

#### Scenario: Off-sale plan

- **WHEN** plan `P` has `for_sale = false`
- **THEN** `P` does NOT appear in the plaza response

### Requirement: Ordering

The system SHALL order model plaza rows primarily by `group.id ASC`, then within each group by row kind (`token` rows before `image` rows), then by model name ascending (case-insensitive). The system SHALL order plan plaza cards by `sort_order ASC, id ASC` (the existing convention on `SubscriptionPlan`).

#### Scenario: Group ordering by id

- **WHEN** two eligible groups exist with `id = 5` and `id = 12`
- **THEN** all rows for group `5` precede all rows for group `12`

#### Scenario: Token rows before image rows within a group

- **WHEN** group `G` has both token-billed and image-generation rows
- **THEN** every token row for `G` appears before every image row for `G`

#### Scenario: Plan cards by sort_order then id

- **WHEN** plans `P1 (sort_order=10, id=3)`, `P2 (sort_order=5, id=7)`, `P3 (sort_order=10, id=2)` are all on sale
- **THEN** the response order is `P2, P3, P1`

### Requirement: Channel Configuration Decoupling

The plaza SHALL ignore all channel-level configuration when computing displayed model rows, including `channel.model_pricing`, `channel.model_mapping`, `channel_model_pricing` overrides, and `channel.restrict_models` allow-lists. The model set is built exclusively from `account.Credentials["model_mapping"]`. Plaza prices represent the list price for a `(group, model)` pair, not the realized billing price for any specific channel.

#### Scenario: Channel-level price override has no effect on plaza

- **WHEN** a channel under group `G` carries a `channel_model_pricing` row that overrides `claude-opus-4-5`'s output price to `$10/Mtok`
- **THEN** the plaza row for `(G, claude-opus-4-5)` still computes `site.output_per_token = LiteLLM_base × G.rate_multiplier`, ignoring the channel override

#### Scenario: Channel model mapping has no effect on plaza model set

- **WHEN** a channel under group `G` declares `claude-opus-4-5 → claude-opus-4-5-internal` in its `model_mapping`, but no account in `G` has `claude-opus-4-5` in its own `model_mapping`
- **THEN** the plaza response does NOT include a row for `(G, claude-opus-4-5)`

### Requirement: Token Price Display Unit

The plaza frontend SHALL render every per-token price using the unit suffix `$<value> / M Tokens` (e.g. `$3.0000 / M Tokens`). The compact form `/Mtok` SHALL NOT appear in any plaza-rendered price string. The same suffix applies to input, output, cache-write and cache-read columns and to both upstream (`base`) and site (`site`) columns. Numeric formatting (decimal places, currency symbol) is governed by the existing `formatUsd` / `formatBase` helpers.

#### Scenario: Input price renders with verbose suffix

- **WHEN** the plaza renders a row whose `input_price_per_mtok = 3.0`
- **THEN** the displayed string is `$3.0000 / M Tokens` (not `$3.0000/Mtok`)

#### Scenario: Cache row renders with same suffix

- **WHEN** the plaza renders a row whose `cache_read_price_per_mtok = 0.3`
- **THEN** the displayed string is `$0.3000 / M Tokens`

#### Scenario: Missing field renders as em-dash

- **WHEN** a row omits `cache_write_price_per_mtok`
- **THEN** the cache-write cell renders as the literal `—` (em dash), not `$0.0000 / M Tokens`

### Requirement: Group-Level Use CTA Contract

Each group block in the plaza model view SHALL render a single primary call-to-action button labelled "Use this group" (i18n key `plaza.use_group`) at the group header. Activating the CTA SHALL navigate to `/keys?openCreate=1&group_id=<group.id>` when the visitor is authenticated, and to `/login?redirect=<encoded /keys URL>` otherwise. After login, `LoginView` SHALL forward the visitor to the encoded redirect target, preserving both `openCreate` and `group_id` query parameters.

#### Scenario: Authenticated user clicks Use this group

- **WHEN** an authenticated user clicks the CTA on group `G` with `id = 42`
- **THEN** the router navigates to `/keys?openCreate=1&group_id=42`

#### Scenario: Anonymous user clicks Use this group

- **WHEN** an anonymous user clicks the CTA on group `G` with `id = 42`
- **THEN** the router navigates to `/login` with a `redirect` query parameter resolving to `/keys?openCreate=1&group_id=42`

#### Scenario: Login redirect preserves nested query

- **WHEN** an anonymous user finishes login on the page reached via `/login?redirect=%2Fkeys%3FopenCreate%3D1%26group_id%3D42`
- **THEN** the post-login redirect lands at `/keys?openCreate=1&group_id=42` with both query parameters intact

### Requirement: Use CTA Triggers Pre-Filled Create-Key Modal

When `KeysView` mounts and `route.query.openCreate === "1"`, the view SHALL automatically open the create-API-key modal with `formData.group_id` set to `Number(route.query.group_id)` (when that value parses to a positive integer matching one of the visitor's selectable groups), and SHALL clear `openCreate` and `group_id` from the URL via a `router.replace` so a page reload does not re-trigger the modal.

#### Scenario: Mount with valid query

- **WHEN** `KeysView` mounts with `route.query = { openCreate: "1", group_id: "42" }` and group `42` is among the visitor's selectable groups
- **THEN** the create-key modal is visible, `formData.group_id === 42`, and the URL is `/keys` (no query)

#### Scenario: Mount with invalid group_id

- **WHEN** `KeysView` mounts with `route.query = { openCreate: "1", group_id: "9999" }` and group `9999` is not selectable for the visitor
- **THEN** the create-key modal opens with the default `formData.group_id` (i.e. unset / first selectable group), and the URL is `/keys`

#### Scenario: Reload after auto-open does not re-trigger

- **WHEN** the create-key modal has been auto-opened and the user reloads the page
- **THEN** the modal does NOT auto-open a second time (because the query was stripped)

### Requirement: Plan Card Buy CTA Contract

Each subscription plan card on the plaza SHALL render a primary call-to-action button labelled "Buy now" (i18n key `plaza.buy_now`) when **and only when** `appStore.cachedPublicSettings.payment_enabled === true`. Activating the CTA SHALL navigate to `/purchase?plan_id=<plan.id>` for authenticated visitors, and to `/login?redirect=<encoded /purchase URL>` for anonymous visitors. When `payment_enabled === false`, the CTA SHALL NOT render at all (no disabled state, no fallback link).

#### Scenario: Buy CTA hidden when payment disabled

- **WHEN** the plaza renders plan cards while `payment_enabled === false`
- **THEN** no plan card displays a "Buy now" button

#### Scenario: Authenticated user clicks Buy now

- **WHEN** `payment_enabled === true`, the user is authenticated, and clicks Buy now on plan `id = 7`
- **THEN** the router navigates to `/purchase?plan_id=7`

#### Scenario: Anonymous user clicks Buy now

- **WHEN** `payment_enabled === true`, the user is anonymous, and clicks Buy now on plan `id = 7`
- **THEN** the router navigates to `/login` with a `redirect` query parameter resolving to `/purchase?plan_id=7`

### Requirement: Buy CTA Pre-Selects Plan on PaymentView

When `PaymentView` finishes initial checkout load and `route.query.plan_id` is a valid integer matching a plan in the loaded checkout AND no Wechat-resume token / state-driven `planId` is in play, the view SHALL set `selectedPlan.value` to that plan and SHALL strip `plan_id` from the URL via `router.replace` to make the entry idempotent.

#### Scenario: Mount with valid plan_id

- **WHEN** `PaymentView` mounts with `route.query = { plan_id: "7" }` and plan `7` exists in the loaded checkout
- **THEN** `selectedPlan.value.id === 7` after `loadCheckout` resolves, and the URL no longer carries `plan_id`

#### Scenario: Mount with invalid plan_id

- **WHEN** `PaymentView` mounts with `route.query = { plan_id: "9999" }` and plan `9999` does not exist
- **THEN** no plan is auto-selected; the URL still has `plan_id` stripped (so a reload starts clean)

#### Scenario: Wechat resume takes precedence

- **WHEN** `PaymentView` mounts with both `route.query.plan_id` and a Wechat-resume token
- **THEN** the existing Wechat-resume flow handles plan selection and the plain-`plan_id` path is a no-op

### Requirement: Homepage Promotes Plaza

The public `HomeView` SHALL provide at least two visible affordances directing visitors to the plaza:
1. A secondary call-to-action button in the hero section labelled "View model pricing" (i18n key `home.cta_view_pricing`), placed alongside the primary "Get Started" CTA.
2. A `PricingTeaser` block rendered before the Features Grid, containing a short headline and a link to `/plaza/models`. The teaser SHALL NOT issue any anonymous API request to populate live data.

The pre-existing top-bar plaza link SHALL be visible on every viewport width (the prior `hidden sm:inline` visibility constraint is removed).

#### Scenario: Hero secondary CTA visible

- **WHEN** an anonymous visitor loads `/`
- **THEN** the hero section contains both a primary "Get Started" button and a secondary "View model pricing" button

#### Scenario: Pricing teaser links to plaza

- **WHEN** an anonymous visitor loads `/`
- **THEN** a teaser block is rendered above the Features Grid; clicking its link navigates to `/plaza/models`

#### Scenario: Top-bar plaza link visible on mobile

- **WHEN** an anonymous visitor loads `/` at viewport width < 640px
- **THEN** the top-bar plaza link is visible (not hidden by responsive utility classes)

#### Scenario: Teaser does not fetch live pricing

- **WHEN** the homepage renders for an anonymous visitor
- **THEN** no request is issued to `/api/v1/plaza/models` from the homepage code path

