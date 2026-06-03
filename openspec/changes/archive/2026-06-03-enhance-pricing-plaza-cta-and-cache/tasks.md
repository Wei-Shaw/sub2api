## 1. Backend — Cache pricing on token rows

- [x] 1.1 Add four optional fields (`CacheWritePricePerMTok`, `CacheReadPricePerMTok`, `SiteCacheWritePricePerMTok`, `SiteCacheReadPricePerMTok`) to `PlazaModelRow` in `backend/internal/service/pricing_plaza_service.go` with `omitempty` JSON tags.
- [x] 1.2 In `buildTokenRow` (the per-MTok scaling block), populate cache fields from `pricing.CacheCreation5mPrice` and `pricing.CacheReadPricePerToken`; multiply by `1_000_000` for `Cache*PerMTok` and additionally by `rate` for `SiteCache*PerMTok`.
- [x] 1.3 Skip emission (leave zero so `omitempty` removes the field) when source value is `0` AND `pricing.SupportsCacheBreakdown == false`. Emit even-if-zero when `SupportsCacheBreakdown == true`.
- [x] 1.4 Mirror the four fields on `PlazaModelRowDTO` in `backend/internal/handler/dto/plaza.go` and update the service→DTO mapper.
- [x] 1.5 Extend `pricing_plaza_service_test.go`:
  - case A: model with `SupportsCacheBreakdown=true` + nonzero cache prices → all four fields present, site = base × rate.
  - case B: model with zero cache prices and `SupportsCacheBreakdown=false` → all four fields absent in JSON.
  - case C: model with explicit zero `CacheCreation5mPrice` but `SupportsCacheBreakdown=true` → field present and equals 0.
- [x] 1.6 Run `go test ./backend/internal/service/... ./backend/internal/handler/...` and `gofmt -l` on touched files; fix any drift.

## 2. Frontend — Plaza model table cache rows + unit format

- [x] 2.1 Update `PlazaModelRow` type in `frontend/src/api/plaza.ts` with the four new optional cache fields (`cache_write_price_per_mtok?`, `cache_read_price_per_mtok?`, `site_cache_write_price_per_mtok?`, `site_cache_read_price_per_mtok?`).
- [x] 2.2 In `frontend/src/components/plaza/ModelPlazaTable.vue`, replace `/Mtok` constant inside `formatTokenPrice` and `formatTokenBase` with ` / M Tokens` (extract a shared `PRICE_UNIT_SUFFIX` constant in the component).
- [x] 2.3 Add two new rows under the existing input/output rows in both base and site columns: cache-write and cache-read, using new i18n keys `plaza.models.cache_write` and `plaza.models.cache_read`. Render `—` when the value is `undefined`.
- [x] 2.4 Verify horizontal overflow on mobile (`whitespace-nowrap` on cells) is preserved with the extra rows.
- [x] 2.5 Add `plaza.models.cache_write` / `plaza.models.cache_read` to `frontend/src/i18n/locales/zh.json` and `en.json` (zh: "缓存写" / "缓存读"; en: "Cache write" / "Cache read").

## 3. Frontend — Auth-aware navigation helper

- [x] 3.1 Create `frontend/src/composables/useAuthRedirect.ts` exporting `gotoOrLogin(target: RouteLocationRaw)` that uses `appStore.isAuthenticated` and `router.resolve(target).fullPath` to build the `/login?redirect=…` URL.
- [x] 3.2 Add a unit test (`useAuthRedirect.spec.ts`) covering: authenticated direct push; anonymous push to `/login` with encoded redirect.

## 4. Frontend — Group-level "Use this group" CTA

- [x] 4.1 In `frontend/src/views/plaza/PlazaModelsView.vue`, render a button in each group block's header (next to the group name / multiplier badge), labelled `t('plaza.use_group')`.
- [x] 4.2 Wire the click handler to `gotoOrLogin({ path: '/keys', query: { openCreate: '1', group_id: String(group.id) } })`.
- [x] 4.3 Add i18n keys `plaza.use_group` (zh: "去使用 ›" / en: "Use this group →").

## 5. Frontend — KeysView query-driven create modal

- [x] 5.1 In `frontend/src/views/user/KeysView.vue`, extend `onMounted` to inspect `route.query.openCreate`. When it equals `'1'`, after the next tick: set `formData.group_id` from `Number(route.query.group_id)` if it parses to a positive integer that exists in `groupOptions`; otherwise leave the default. Set `showCreateModal.value = true`.
- [x] 5.2 Immediately call `router.replace({ path: '/keys' })` (without query) so reload doesn't re-trigger.
- [x] 5.3 Manually verify the data-tour for `keys-create-btn` still works (no race with auto-open).

## 6. Frontend — Plan card "Buy now" CTA

- [x] 6.1 In `frontend/src/components/plaza/PlanPlazaCards.vue`, render a primary button labelled `t('plaza.buy_now')` on each card, but ONLY when `appStore.cachedPublicSettings.payment_enabled === true`.
- [x] 6.2 Wire the click handler via `gotoOrLogin({ path: '/purchase', query: { plan_id: String(plan.id) } })`.
- [x] 6.3 Add i18n keys `plaza.buy_now` (zh: "立即购买" / en: "Buy now").

## 7. Frontend — PaymentView query-driven plan pre-select

- [x] 7.1 In `frontend/src/views/user/PaymentView.vue`, after `loadCheckout()` resolves and after the existing Wechat-resume / state-driven blocks have decided not to act, inspect `route.query.plan_id`. If it parses to a valid integer matching a `checkout.value.plans[*].id`, set `selectedPlan.value` to that plan.
- [x] 7.2 In all branches (matched, unmatched, not present) call `router.replace({ path: route.path, query: <query without plan_id> })` so reloads remain clean.
- [x] 7.3 Confirm the existing `parseWechatResumeRoute` flow still wins precedence (mount-time check guards on Wechat-resume signals).

## 8. Frontend — Homepage promotion

- [x] 8.1 In `frontend/src/views/HomeView.vue`, add a secondary outline button "View model pricing" inside the hero CTA row, next to "Get Started". Use new i18n key `home.cta_view_pricing`. Link to `/plaza/models`.
- [x] 8.2 Create `frontend/src/components/home/PricingTeaser.vue` (small banner, no API call) with i18n keys `home.pricing_teaser_title` and `home.pricing_teaser_link`. Render it in `HomeView` immediately above the Features Grid.
- [x] 8.3 Remove `hidden sm:inline` from the existing top-bar plaza `<router-link>` so it stays visible on mobile.
- [x] 8.4 Add the new home i18n keys to both `zh.json` and `en.json`.

## 9. Verification

- [x] 9.1 `gofmt -l` and `golangci-lint run ./backend/internal/service/... ./backend/internal/handler/...` show no new issues on touched files.
- [x] 9.2 `cd frontend && pnpm typecheck && pnpm test` pass (incl. new `useAuthRedirect` spec).
- [ ] 9.3 Manual smoke test in dev:
  - `/plaza/models` shows 4 rows per model where data exists; `—` rendered where unknown; suffix is `$x / M Tokens`.
  - Click "Use this group" while logged out → land on `/login` with redirect → after login lands on `/keys` with create modal open and group preselected; URL stripped to `/keys`.
  - Click "Buy now" while logged out (with `payment_enabled=true`) → after login lands on `/purchase` with plan preselected.
  - With `payment_enabled=false`, plan cards have no "Buy now" button.
  - Homepage shows hero "View model pricing" button + teaser block; mobile top-bar shows the plaza link.
- [x] 9.4 Run `openspec validate enhance-pricing-plaza-cta-and-cache --strict` and confirm pass.
