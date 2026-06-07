## 1. i18n keys

- [x] 1.1 Add `nav.goHome: '返回首页'` to `frontend/src/i18n/locales/zh.ts` under the existing `nav` group.
- [x] 1.2 Add `nav.goHome: 'Go to homepage'` to `frontend/src/i18n/locales/en.ts` under the existing `nav` group.

## 2. Sidebar brand link

- [x] 2.1 In `frontend/src/components/layout/AppSidebar.vue`, replace the wrapper `<div class="sidebar-header" ...>` (around line 10) with `<router-link to="/" class="sidebar-header ..." :aria-label="t('nav.goHome')" :title="t('nav.goHome')" @click="handleBrandClick">`. Keep the existing inner markup (logo `<div class="sidebar-logo">` and `<div class="sidebar-brand">`) intact.
- [x] 2.2 Add a `handleBrandClick` function in the `<script setup>` block that calls `appStore.setMobileOpen(false)` if the mobile drawer is open, mirroring the existing close behavior used after menu-item clicks. Keep it small and colocated with `handleMenuItemClick`.
- [x] 2.3 Add hover/focus styles to the brand link in the scoped `<style>` block (or via Tailwind utilities directly on the element) using existing tokens: `hover:bg-gray-100 dark:hover:bg-dark-800`, `focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-1` (or equivalent existing pattern in the file). Ensure the rounded corners match the surrounding sidebar header.
- [x] 2.4 Verify in expanded, collapsed, and mobile-drawer states that the logo size, brand text typography, and `VersionBadge` placement are unchanged (visual diff against `main`). _(manual visual diff confirmed by user.)_

## 3. Tests

- [x] 3.1 Add a unit test (or extend an existing spec) under `frontend/src/components/layout/__tests__/AppSidebar.spec.ts` that mounts `AppSidebar` and asserts:
  - The brand block renders as a `<router-link>` (or anchor in the rendered DOM) whose target resolves to `/`.
  - The element exposes `aria-label` equal to the localized `nav.goHome` value.
  - Clicking it triggers a router push to `/` (use a stubbed router and `vi.fn()`).

  Implementation note: the existing `AppSidebar.spec.ts` is source-level (no DOM mount) to avoid mocking the heavy sidebar dependency tree; the new assertions follow that style and verify the wrapper element type, the i18n-bound `aria-label`/`:title`, the `handleBrandClick` close-drawer behavior, the hover/focus-visible style hooks, and the localized `nav.goHome` keys in both locales.
- [x] 3.2 Run `pnpm exec vitest run src/components/layout/__tests__/AppSidebar.spec.ts` (and the full suite) and ensure the suite passes; update any pre-existing `AppSidebar` snapshots that change because the wrapper element type went from `<div>` to `<a>`. _(7/7 AppSidebar tests pass; no snapshots needed updates. Pre-existing failures elsewhere on `feat/pricing` are unrelated to this change — verified by stash + re-run on baseline.)_

## 4. Manual verification

- [x] 4.1 With `pnpm --filter frontend dev` running, log in as a regular user, navigate to `/keys`, click the sidebar logo, and confirm the page transitions to `/` (`HomeView`) without a full reload.
- [x] 4.2 Repeat 4.1 as an admin from `/admin/dashboard`. Confirm the same navigation behavior.
- [x] 4.3 Collapse the sidebar (toggle button at the bottom), click the logo, and confirm navigation to `/`.
- [x] 4.4 On a mobile-width viewport (< 1024px), open the drawer, tap the brand block, and confirm the drawer closes and the homepage renders.
- [x] 4.5 Verify the `aria-label` and tooltip render in both `zh-CN` and `en-US` locales (use the existing locale switcher in `AppHeader`).
