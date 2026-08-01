# Dashboard Redesign + Orange Rebrand — Design

- **Date:** 2026-07-29
- **Status:** Draft (pending user review)
- **Owner:** frontend
- **Reference:** https://www.jiji.cc/dashboard (logged-in user dashboard)

## 1. Background & Goal

The deployed site is currently inaccessible (ICP filing pending), and the operator wants to modernize the **user dashboard** (`/dashboard`) using jiji.cc as the visual/structural reference, plus rebrand the **whole app** from teal to jiji.cc's orange.

The current user dashboard is information-dense (8 metric cards + per-platform breakdown) and lacks a developer onboarding element. jiji.cc is cleaner: a 4-card metric row, a prominent **"快速接入" (Quick Access)** card showing the API base URL + key, a 4-cell quick-entry grid, recent calls, and a model-cost bar chart.

**Goal:** adopt jiji.cc's skeleton (greeting, 4 metrics, Quick Access card, quick-entry grid) while keeping Sub2API's deeper analytics (trend chart with date/granularity, per-platform breakdown + quotas), and switch the global primary color to orange.

## 2. Scope

**In scope:**
- Global primary color rebrand: teal → orange (app-wide).
- User dashboard `/dashboard` structural reorganization.
- New "快速接入" card component.
- Quick-entry vertical list → 4-cell grid.
- Move 4 secondary metrics into the trend chart card.
- Fix the Tailwind `@apply` build blocker (prerequisite, Phase 0).

**Out of scope:**
- Admin dashboard (`/admin/dashboard`), landing page (`/home`), and all other routes' *layout* (they only inherit the color change).
- Backend/API changes (all needed data already exists — see §6).
- Sidebar menu item changes (current nav stays).
- jiji.cc features Sub2API has no equivalent for (e.g., "网页对话" chat, "权益兑换" as a distinct concept) — quick-entry cards link to existing Sub2API routes only.

## 3. Design

### 3.0 Phase 0 — Fix `@apply` build blocker (prerequisite)

The working-copy frontend cannot `pnpm dev` / `pnpm build`. Tailwind throws `The \`X\` class does not exist` for `@apply` usages involving custom colors, e.g.:
- `@apply dark:text-dark-200` — the `dark:` **variant** collides with a color literally named **`dark`**.
- `@apply bg-primary-500/20` — custom color + opacity modifier in `@apply`.

**Fix approach (to be confirmed at implementation):**
- Replace colliding `@apply <util>-dark-<shade>` with the hex-identical `slate-<shade>` (already proven: dark-* and slate-* share the same values in `tailwind.config.js`).
- For custom-color `@apply` with opacity/variant that still fails, move those declarations out of `@apply` into raw CSS (or into template class attributes).

**Verification:** `pnpm build` completes green; `pnpm dev` serves with no PostCSS overlay.

### 3.1 Global — primary color teal → orange

Replace the `primary` palette in `frontend/tailwind.config.js` (currently teal, `#14b8a6`-based) with a warm-orange palette anchored at jiji.cc's `#FF6B35`:

| shade | hex (proposed) |
|------|-----------------|
| 50   | #FFF4ED |
| 100  | #FFE6D5 |
| 200  | #FFC9A8 |
| 300  | #FFA570 |
| 400  | #FF7E3D |
| 500  | #FF6B35 |
| 600  | #F04E1A |
| 700  | #C73C13 |
| 800  | #9E3116 |
| 900  | #7F2B16 |
| 950  | #45140A |

**Impact:** every `primary-*` usage app-wide (buttons, links, focus rings, active nav, accents) becomes orange automatically. The `accent` palette stays as-is (it is a neutral slate duplicate). Audit step: grep for hardcoded teal hex (`#14b8a6`, `#0d9488`, etc.) and `teal-` utilities and convert them to `primary-*` or the new orange hex.

### 3.2 Dashboard `/dashboard` — new structure (top → bottom)

`views/user/DashboardView.vue` is reorganized into a single-column flow (drops the current 2/3 + 1/3 split):

1. **Greeting header** — `概览` title + `「{username}，这里是你的账户与 API 使用概况」`.
2. **4 metric cards** (`UserDashboardStats`, slimmed) — 账户余额 / 今日消费(actual) / 今日调用(today_requests) / 接入密钥(active/total).
3. **🆕 Quick Access card** (`UserDashboardQuickAccess.vue`, new) — see §3.3.
4. **Trend chart** (`UserDashboardCharts`, kept) — plus the 4 moved metrics as small auxiliary text in the card header/footer (see §3.4).
5. **Quick-entry 4-grid** (`UserDashboardQuickActions.vue`, restyled from vertical list to grid) — 充值 / 兑换码 / 管理 API 密钥 / 批量生图.
6. **Recent calls table** (`UserDashboardRecentUsage`, kept) — add 「查看完整账单 →」 link to `/usage`.
7. **Model usage distribution** (kept).
8. **Per-platform breakdown + quotas** (kept from current `UserDashboardStats` Row 3) — moved to the bottom as a secondary region so it does not dominate the first screen.

### 3.3 New component — `UserDashboardQuickAccess.vue`

A card that surfaces the developer quickstart (jiji.cc's signature element):
- **Base URL:** from `public_settings.api_base_url`; fallback `window.location.origin + '/v1'`.
- **Authorization:** masked preview of the user's **first active API key** (`keysAPI.list({ status: 'active' })` → first item's masked preview, e.g. `sk-xxxx1234`). If no active key, show a 「去创建」 CTA linking to `/keys`.
- **复制地址** button — copies the Base URL to clipboard.
- **配置教程** link — to the existing docs/help route (verify which; fallback `/keys` or docs URL from public settings).
- **服务正常** green-dot status indicator — **static green** (no health probe in scope; purely visual, matches jiji.cc).

### 3.4 Move 4 secondary metrics into the trend chart card

The 4 cards removed from the top row — 今日Token / 总Token / RPM·TPM / 平均响应 — are shown as compact auxiliary figures in the **trend chart card** (e.g., a small stat strip beneath the title): `今日Token · 总Token · RPM · TPM · 平均响应`. Data already in `stats` (`today_tokens`, `total_tokens`, `rpm`, `tpm`, `average_duration_ms`).

### 3.5 i18n

Add new keys under `dashboard.*` in `frontend/src/i18n/locales/{zh,en}/dashboard.ts` (and `common.ts` as needed): greeting title/subtitle, quick access labels (base URL, authorization, copy, tutorial, service ok), "去创建", "查看完整账单". English + Chinese.

## 4. Component touch-list

| File | Change |
|---|---|
| `tailwind.config.js` | `primary` → orange palette; audit teal leftovers |
| `views/user/DashboardView.vue` | New single-column layout order; add greeting + QuickAccess |
| `components/user/dashboard/UserDashboardStats.vue` | Slim to 4 top cards; move Row 2 (4 metrics) out; keep platform breakdown as separate block |
| `components/user/dashboard/UserDashboardQuickAccess.vue` | **NEW** — quick access card |
| `components/user/dashboard/UserDashboardCharts.vue` | Add 4-metric aux strip in card |
| `components/user/dashboard/UserDashboardQuickActions.vue` | Vertical list → 4-cell grid |
| `components/user/dashboard/UserDashboardRecentUsage.vue` | Add 「查看完整账单 →」 link |
| `i18n/locales/{zh,en}/dashboard.ts` | New keys |
| `src/**/*.vue` + `src/style.css` | Phase 0 `@apply` fixes |

## 5. Data / API (all existing — no backend work)

- Stats: `usageAPI.getDashboardStats()` — has balance sources, today_requests, today_actual_cost, total/active api_keys, today/total tokens, rpm/tpm, average_duration_ms.
- Base URL: `public_settings.api_base_url` (`window.__APP_CONFIG__` / `appStore.cachedPublicSettings`).
- First active key: `keysAPI.list({ page:1, page_size:1, status:'active' })` — masked preview.
- Username: `authStore.user`.

## 6. Risks / Open Questions

- **Global orange reach:** changing `primary` affects the entire app (every page's buttons/links/nav). Intended per user ("全部改橙色"), but first preview will surface any visually broken spots.
- **Key masking vs copy:** the key list API returns masked keys; "复制" therefore copies the **Base URL** (not the secret), matching jiji.cc. If a copyable full key is later wanted, that needs a separate reveal API (out of scope).
- **@apply root cause:** the exact reason custom-color `@apply` fails is to be confirmed in Phase 0; the fix is verified by a green `pnpm build`.
- **"配置教程" target:** confirm the docs/help route to link (fallback: `/keys`).

## 7. Implementation phases (high-level)

0. Fix `@apply` build blocker → `pnpm build` green.
1. Global primary → orange; audit hardcoded teal.
2. Dashboard restructure: greeting, slim 4-card stats, move 4 metrics to trend card, platform-breakdown to bottom.
3. New QuickAccess card + data wiring.
4. QuickActions → 4-grid; RecentUsage "view full bill" link.
5. i18n; visual QA in `pnpm dev`; verify against the local running backend on `:8080`.
