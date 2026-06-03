## Context

`pricing-plaza` shipped as a read-only browse surface (capability `pricing-plaza`, archived 2026-06-02). Two gaps surfaced as soon as it went live:

1. **Pricing completeness.** Anthropic-style models bill prompt-cache reads/writes at very different rates from input/output tokens. The plaza only renders input/output, so the displayed offer is misleading for the highest-volume models. `ModelPricing` already carries cache prices (`CacheCreation5mPrice`, `CacheCreation1hPrice`, `CacheReadPricePerToken`, `SupportsCacheBreakdown`); the plaza just doesn't surface them. Cache columns also reveal that the abbreviated `/Mtok` unit reads as noise to non-technical visitors — `$x / M Tokens` is what the rest of the industry uses.
2. **Funnel break.** The plaza is meant to be a top-of-funnel — visitors who pick a group/model or a plan have to hunt for the right authenticated page. The user-facing UX needs the plaza to *commit* the visitor to a next action (create an API key in this group / buy this plan), with the auth wall handled transparently. Current code already supports the building blocks: `LoginView` reads `?redirect=`; `PaymentView` already understands `plan_id` (used by Wechat resume). `KeysView` opens a create modal driven by `showCreateModal` + `formData.group_id`; nothing yet wires query-string entry.
3. **Discovery.** `HomeView` has only one small `<router-link>` to the plaza, hidden on mobile (`hidden sm:inline`). New visitors don't know it exists.

Stakeholders: anonymous prospects (primary audience for plaza), authenticated users (target after CTA), site admins (don't want broken CTAs when payment is disabled).

## Goals / Non-Goals

**Goals**
- Plaza model row displays all four headline prices (input / output / cache write / cache read) with consistent `$x / M Tokens` formatting; missing cache data renders as `—`, never as `$0.0000`.
- Group block has a single primary CTA leading anonymous visitors through login back to the create-key modal pre-selected to that group.
- Plan card CTA leads (anonymous-via-login or directly) to `/purchase` with the plan pre-selected, and is hidden when payments are globally disabled.
- Homepage advertises the plaza both in the hero CTA and in a dedicated "transparent pricing" teaser block above features grid.
- All routing is query-string driven, idempotent, and survives reload.

**Non-Goals**
- 1-hour cache pricing tier (5m only — covers the headline use case for v1).
- Channel-level price overrides / per-channel mapping in plaza output.
- Live plaza-data fetch on the homepage teaser (static copy + link only — avoids spinners on a marketing page and an extra anonymous query each visit).
- Per-row "use" CTA inside the table (decision: group-header CTA only).
- Deep linking back to the plaza from `/keys` or `/purchase` after success.
- Showing a per-row toggle when `payment_enabled=false` for buttons unrelated to subscriptions (only plan cards are affected).

## Decisions

### D1. Cache pricing — single tier, sourced from existing `ModelPricing.CacheCreation5mPrice`

`PlazaModelRow` (and DTO + frontend type) gains:

```go
CacheWritePricePerMTok      float64 `json:"cache_write_price_per_mtok,omitempty"`
CacheReadPricePerMTok       float64 `json:"cache_read_price_per_mtok,omitempty"`
SiteCacheWritePricePerMTok  float64 `json:"site_cache_write_price_per_mtok,omitempty"`
SiteCacheReadPricePerMTok   float64 `json:"site_cache_read_price_per_mtok,omitempty"`
```

In `buildTokenRow` (at the existing per-MTok scaling block), populate from `pricing.CacheCreation5mPrice` and `pricing.CacheReadPricePerToken`, multiplied by `1_000_000` and (for site columns) by `rate`. **Zero values are emitted as zero, but the frontend distinguishes between `0` (free / not applicable) and `undefined` (unknown). To preserve "unknown → render —", we treat `0` as `undefined` for cache columns specifically** — most fallback OpenAI models report `0`, and rendering `$0` would be misleading. Implementation: when the source `ModelPricing` returns `0` for a cache field AND `SupportsCacheBreakdown == false`, omit the JSON field (`omitempty` does this naturally since zero is skipped).

**Alternatives rejected:**
- 5m + 1h dual tier — only Anthropic exposes 1h, would force a wider table; revisit if user demand emerges.
- Embed cache in a `prices.token.cache` sub-object — would break the existing flat model row schema for marginal benefit.

### D2. Unit format — `$x / M Tokens`

Replace `/Mtok` in `formatTokenPrice` and `formatTokenBase` with ` / M Tokens`. Decimal places stay at 4 for input/output; cache columns also use 4 (cache prices can be small but `$0.3000 / M Tokens` for cache-read on Sonnet is still readable). Single source — both helpers take the suffix from a constant `PRICE_UNIT_SUFFIX = ' / M Tokens'`.

### D3. CTA URL contract

```
GET /keys?openCreate=1&group_id=<id>
GET /purchase?plan_id=<id>
```

`openCreate=1` is required because we want to permit `/keys?group_id=…` someday for filter-only entry. `KeysView.onMounted`:

```ts
if (route.query.openCreate === '1') {
  formData.group_id = Number(route.query.group_id) || formData.group_id
  showCreateModal.value = true
  router.replace({ path: route.path, query: {} })
}
```

`PaymentView` already handles `plan_id` for the Wechat resume code path (`parseWechatResumeRoute`). We add a parallel, lighter mount-time hook: when `route.query.plan_id` is present **and** no Wechat resume token / state-driven `planId` is in play, set `selectedPlan.value` to the matching plan after `loadCheckout()` resolves, then strip `plan_id` from the URL.

**Alternative rejected:** opaque session token instead of plain `group_id` / `plan_id`. Plaza data is already public, the IDs are already returned in the public plaza API, no marginal benefit.

### D4. Auth-aware navigation helper

Frontend reuses `appStore.isAuthenticated`:

```ts
function gotoOrLogin(target: RouteLocationRaw) {
  if (appStore.isAuthenticated) {
    return router.push(target)
  }
  // vue-router will URL-encode the nested query
  return router.push({ path: '/login', query: { redirect: router.resolve(target).fullPath } })
}
```

The helper lives in a small composable `useAuthRedirect.ts` shared between `PlazaModelsView` (group CTA) and `PlanPlazaCards` (buy CTA). After login, `LoginView` does `router.push(redirectTo)` with the already-encoded `fullPath`, which preserves the nested query string.

**Alternative rejected:** raw `query: { redirect: '/keys?openCreate=1&group_id=42' }`. Vue Router itself encodes the query value, but `LoginView` reads it as `string` and `router.push(string)` accepts a path-with-query, so this works without manual `encodeURIComponent`. We just have to verify `redirect` is a string, not an array, when LoginView consumes it (already the case in current code).

### D5. `payment_enabled=false` gating for plan card CTA

`PlanPlazaCards` consumes `appStore.cachedPublicSettings.payment_enabled` (already populated for anonymous visitors). When false, omit the CTA entirely (don't render disabled state — clutter on a marketing card). Independent of auth state.

### D6. Homepage promotion — minimal, static, no API call

Hero CTA section adds a secondary outline button **alongside** `Get Started`:

```
[ Get Started ]   [ View model pricing → ]
```

Below the hero (still in HomeView), insert a `PricingTeaser.vue` component before the Features Grid:

```
─────────────────────────────────────────────────
| 💰 Transparent pricing — pay per token        |
| Browse all models and plans in the plaza →    |
─────────────────────────────────────────────────
```

No API call. The "from $X / M Tokens" claim copy is **omitted** (would need live data or a hardcoded number we'd have to keep in sync). Top-bar nav-link drops `hidden sm:inline` — replaced with always visible.

## Risks / Trade-offs

- **Cache columns inflate table width on narrow screens** → Existing table already has overflow-x scroll on mobile (verified by re-reading `ModelPlazaTable.vue` markup); add column wrap with `whitespace-nowrap` and let mobile users scroll horizontally.
- **`KeysView` mount-side modal opening clashes with auth-stage tour highlight** (`data-tour="keys-create-btn"`) → Add a one-tick `nextTick` before opening the modal so tour init doesn't race; clear query params *after* opening to avoid double-trigger on HMR.
- **Login round-trip can lose form state if user 3rd-party-OAuths back to a different domain** → Out of scope for this change; existing flows have the same property.
- **Anonymous user clicking `[Buy now]` while `payment_enabled` is true but later disabled before they finish login** → They land on `/purchase`, which is router-guarded with `requiresPayment: true` and will redirect to dashboard. Acceptable — this is a transient race, not a regression.
- **Cache-write/-read column for OpenAI models showing `—` while OpenAI does charge a cache-read rate at the API layer** → We render only what `ModelPricing` (built from LiteLLM/fallback) tells us. If LiteLLM omits cache fields for a model, `—` is more honest than zero. Revisit when LiteLLM data covers more providers.
- **`/Mtok` → ` / M Tokens` is a copy change visible in screenshots / docs** → We sweep both places, but keep the constant in one helper file so further tweaks are one-line.

## Migration Plan

1. Backend DTO is additive (new optional fields). No breaking client change — older frontends will simply ignore the extra fields.
2. Frontend ships in the same release; older cached HTML clients keep working but won't render cache rows or CTAs until they pick up the new bundle.
3. No DB migration. No config change. No env vars.
4. Rollback: revert the merge — plaza falls back to two-row token display and read-only cards.

## Open Questions

- None blocking. (Resolved: single-tier cache, group-level CTA, hide-on-disabled, hero+teaser homepage layout.)
