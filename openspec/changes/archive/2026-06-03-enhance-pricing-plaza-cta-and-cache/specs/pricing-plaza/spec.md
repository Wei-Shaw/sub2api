## MODIFIED Requirements

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

## ADDED Requirements

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
