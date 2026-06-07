## Why

Inside the authenticated console (any page rendered under `AppLayout`), the brand block at the top of `AppSidebar` (logo + site name + version badge) is presentational only — it cannot be clicked. Returning to the public homepage today requires either editing the URL, opening a new tab, or logging out, which is the standard escape hatch users expect from clicking a brand logo. Making the brand block a link to `/` matches near-universal web conventions and gives users an obvious way to reach the marketing/plaza homepage from inside the console.

## What Changes

- Make the sidebar brand block (`<div class="sidebar-header">` and its logo + brand text in `frontend/src/components/layout/AppSidebar.vue`) navigate to the public homepage `/` when clicked.
  - The whole brand block (logo image **and** site name) becomes a single clickable target so the click area is generous and predictable, regardless of `sidebarCollapsed` state.
  - When the sidebar is collapsed (icon-only), clicking the logo still navigates to `/`.
- Use Vue Router's `<router-link to="/">` to integrate with SPA navigation (no full page reload, preserves history) instead of `<a href="/">`.
- Add a hover/focus affordance (subtle background or opacity change) and an accessible name (e.g. `aria-label="Go to homepage"` / i18n key `nav.goHome`) so screen-reader and keyboard users understand the new affordance.
- The existing visual layout (logo size, brand text, version badge, collapsed state) MUST NOT regress.

## Capabilities

### New Capabilities
- `console-navigation`: Brand-level navigation affordances inside the authenticated console layout (sidebar header, top-bar). Establishes the contract that the brand/logo serves as a link to the public homepage.

### Modified Capabilities
<!-- None — this introduces a fresh capability; no existing spec governs the sidebar brand affordance. -->

## Impact

- **Frontend**:
  - `frontend/src/components/layout/AppSidebar.vue` — wrap brand block in `<router-link>`, add hover state and `aria-label`.
  - `frontend/src/i18n/locales/zh.ts` and `frontend/src/i18n/locales/en.ts` — add a single i18n key (e.g. `nav.goHome` → "返回首页" / "Go to homepage") used as `aria-label` and tooltip.
- **Backend**: None.
- **Routing/auth**: The target route `/` is `HomeView`, which is already public-accessible — no router-guard changes required. Authenticated users hitting `/` continue to see `HomeView` (current behavior); no automatic redirect is added or removed.
- **Tests**: Add a small unit test for `AppSidebar.vue` asserting the brand block renders as a router-link to `/` and exposes the accessible name.
