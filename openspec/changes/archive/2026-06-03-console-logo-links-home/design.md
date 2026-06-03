## Context

Inside the authenticated console layout (`AppLayout`), `frontend/src/components/layout/AppSidebar.vue` renders a fixed brand block at the top of the sidebar — a logo image plus the site name and a `VersionBadge`. This block is purely presentational today (`<div>`-only). The public homepage `/` (`HomeView`) acts as the marketing/plaza entry point, but there is no in-console affordance to return to it short of typing the URL or signing out. The change adds the missing affordance by turning the brand block into a `<router-link to="/">`.

The sidebar is shared by both regular users and admins, and supports a collapsed (icon-only) state and a mobile drawer state via `useAppStore()`. Any change to the brand block must work uniformly across these states without breaking existing styles.

## Goals / Non-Goals

**Goals:**
- Allow users to return to the public homepage `/` by clicking the brand block from any console page.
- Maintain existing visual layout in expanded, collapsed, and mobile-drawer states.
- Provide an accessible name and clear hover/focus state so the new affordance is discoverable.
- Use SPA navigation (no full reload) and respect the existing router.

**Non-Goals:**
- Add a logo to `AppHeader.vue` (the top bar already has its own page-title slot; not in scope).
- Change the destination based on auth state (e.g. send admins to `/admin/dashboard` instead). The user explicitly asked for "首页", which we interpret as `/`.
- Restyle the brand block (sizes, colors, version badge) — only the wrapper element and an `aria-label` change.
- Auto-close the mobile sidebar drawer on click. Optional polish; the existing `handleMenuItemClick` handler is reused via a tiny shared close helper if straightforward, otherwise treated as out-of-scope and left to a follow-up.

## Decisions

### Decision 1: Use `<router-link to="/">` instead of `<a href="/">`
**Choice:** Wrap the existing brand block (`logo` + `brand` text divs) in a single `<router-link to="/" class="sidebar-header ..." aria-label="...">`.

**Rationale:** Vue Router-aware navigation avoids a full page reload, preserves history correctly, and is consistent with every other navigation entry in the sidebar (which all use `<router-link>`). `<a href="/">` would force a hard reload and lose the SPA state, which is jarring.

**Alternatives considered:**
- Keep `<div>` and bind `@click="router.push('/')"` — rejected: less semantically correct, no built-in keyboard activation (Enter/Space), and harder to expose accessibility correctly.
- Make only the `<img>` clickable — rejected: collapsed-state click target would be ~36×36px and the brand text would remain non-interactive, which is inconsistent with web conventions.

### Decision 2: Single wrapping link covers both logo and brand text
The whole `.sidebar-header` div becomes the link. Both expanded and collapsed states reuse the same target. The brand text is hidden via CSS (`sidebar-brand-collapsed`) when collapsed, but the link element still spans the logo, so the click target stays generous.

### Decision 3: Accessible name via `aria-label` (i18n)
Add a single i18n key `nav.goHome` ("返回首页" / "Go to homepage"). Bind it to `aria-label` on the link and to a `:title` attribute (native tooltip). The visible logo + brand text remain the primary affordance for sighted users; the `aria-label` plus a `:title` cover assistive tech and hover discoverability.

### Decision 4: Hover/focus state
Apply a subtle Tailwind hover (`hover:bg-gray-100 dark:hover:bg-dark-800`) and a visible focus ring (`focus-visible:ring-2 focus-visible:ring-primary-500`) on the wrapping link. Reuse existing color tokens; do not introduce new ones.

### Decision 5: No auth-state branching
Even when an admin is logged in, clicking the logo lands on `/` (`HomeView`). `HomeView` already adapts to authenticated visitors (the user dropdown / hero CTA shift accordingly), so no special-casing is needed in this change. If product later wants admins to land on `/admin/dashboard` instead, that is a separate change.

## Risks / Trade-offs

- **Risk:** Mobile sidebar may stay open after navigating, leaving a visible drawer over the homepage. → **Mitigation:** Reuse the existing pattern in `handleMenuItemClick` by calling `appStore.setMobileOpen(false)` from a small `onClick` handler on the brand link (or via an inline handler). Cheap to implement; included in tasks.
- **Risk:** Snapshot or layout tests in `AppSidebar` may break because the wrapping element changes from `<div>` to `<a>`. → **Mitigation:** Run `frontend` tests after the edit; update any affected snapshots/selectors. The existing CSS (`.sidebar-header`, `.sidebar-logo`, `.sidebar-brand`) is class-based and unaffected by the element-type change.
- **Risk:** Admins on `/admin/*` who were used to the brand block being inert might reflexively click and lose context. → **Mitigation:** Acceptable trade-off — this matches universal web convention and the destination is a static, low-risk page; users can navigate back via browser history.
- **Risk:** New `aria-label`/title pair in two locales drifts in translation. → **Mitigation:** Single short key `nav.goHome` colocated with other nav keys; reviewed in PR.

## Migration Plan

No migration required. Front-end-only change shipped behind no flag. Rollback is a one-file revert of `AppSidebar.vue` and the two i18n locale files.
