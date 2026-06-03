## Why

The Pricing Plaza shipped as a read-only marketing surface, but visitors who decide on a model or plan still have to navigate manually to the right user pages — that creates funnel drop-off, and the model rows are missing prompt-cache pricing that materially changes Anthropic / Claude economics. We also want the public homepage to actually advertise the plaza so newcomers find it.

## What Changes

- Plaza model row exposes prompt-cache pricing
  - Backend `PlazaModelRow` adds `cache_write_price_per_mtok`, `cache_read_price_per_mtok`, `site_cache_write_price_per_mtok`, `site_cache_read_price_per_mtok` (single-tier, sourced from existing `ModelPricing.CacheCreation5mPrice` / `CacheReadPricePerToken`).
  - Frontend `ModelPlazaTable` renders 4 lines per model (input / output / cache-write / cache-read) using `$x / M Tokens` (replaces the abbreviated `/Mtok`).
  - Missing cache pricing renders as `—`.
- Plaza becomes actionable
  - Each group block in `PlazaModelsView` gets a group-level **去使用 / Use this group** button that routes to `/keys?openCreate=1&group_id=<id>`. `KeysView` parses these query params on mount, opens the create-key modal pre-selected to that group, then strips the params.
  - Each plan card in `PlazaPlansView` gets a **立即购买 / Buy now** button routing to `/purchase?plan_id=<id>`. `PaymentView` already accepts `plan_id`; ensure pre-selection on mount works for query-string entry, not only `state`.
  - Buttons are hidden on plan cards when `appStore.cachedPublicSettings.payment_enabled === false`.
  - Both CTAs honor auth: unauthenticated visitors are routed through `/login?redirect=<encoded target>` (LoginView already supports the `redirect` query).
- Homepage promotes the plaza
  - Hero CTA row gains a secondary `[查看模型价格 / View model pricing]` button next to `[Get Started]`.
  - A new `PricingTeaser` section is inserted before Features Grid showing 1–2 line copy ("Transparent pricing — from $X / M Tokens") plus a `[浏览所有模型 →]` link to `/plaza/models`. No live data fetch in v1 (avoid empty-state flicker on slow networks).
  - Top-bar plaza link drops `hidden sm:inline` so mobile users see it too.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `pricing-plaza`: model-row schema gains cache-write/cache-read fields and uses `$x / M Tokens` in clients; plaza views expose group-level "use" CTA and plan-level "buy" CTA with login-redirect handling and `payment_enabled` gating; homepage gains a plaza promotion surface.

## Impact

- Backend
  - `internal/service/pricing_plaza_service.go`: extend `PlazaModelRow` + token-price assembly to carry cache prices.
  - `internal/handler/dto/plaza.go`: 4 new optional fields on `PlazaModelRowDTO`.
  - Tests: `pricing_plaza_service_test.go` adds cases for cache-bearing model + missing cache prices.
- Frontend
  - `src/api/plaza.ts` types updated.
  - `src/components/plaza/ModelPlazaTable.vue`: 2 new rows per model; format helper migrates to `$x / M Tokens`.
  - `src/views/plaza/PlazaModelsView.vue`: group header CTA, auth-aware navigation helper.
  - `src/components/plaza/PlanPlazaCards.vue`: buy CTA with `payment_enabled` gate.
  - `src/views/user/KeysView.vue`: read `openCreate` / `group_id` query → open modal preselected.
  - `src/views/user/PaymentView.vue`: ensure mount-time `route.query.plan_id` triggers the same selection path as `state.planId`.
  - `src/views/HomeView.vue`: hero secondary CTA + `PricingTeaser` block + nav-link visibility tweak.
  - i18n (zh/en): new strings `plaza.use_group`, `plaza.buy_now`, `plaza.cache_write`, `plaza.cache_read`, `home.cta_view_pricing`, `home.pricing_teaser_*`.
- Out of scope
  - 1-hour cache tier display, channel-level overrides, live homepage pricing fetch, post-purchase deep linking back to plaza.
