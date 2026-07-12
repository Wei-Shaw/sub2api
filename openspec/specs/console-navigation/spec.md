# Console Navigation

## Purpose

Define the navigation affordances inside the authenticated console shell (`AppLayout` + `AppSidebar`), so that users can move between the console and the public surfaces (homepage, plaza) with predictable, accessible, SPA-only transitions. This capability covers the sidebar brand block and any future cross-surface navigation primitives that live inside the console chrome.

## Requirements

### Requirement: Sidebar Brand Block Links to Homepage

The console sidebar (`AppSidebar` component, rendered for every authenticated route under `AppLayout`) SHALL render its brand block — the logo image together with the site name — as a single navigational link whose target is the public homepage `/`. Activating this link (mouse click, Enter, or Space) MUST perform an SPA navigation (no full page reload) to `/`.

The link element MUST:
- Use Vue Router (`<router-link to="/">` or equivalent) so navigation history and route guards behave consistently with the rest of the sidebar.
- Cover both the logo image and the brand text as a single click target in the expanded state, and remain clickable on the logo in the collapsed (icon-only) state.
- Expose an accessible name via `aria-label` (and a matching native `title` tooltip) sourced from a localized i18n key (e.g. `nav.goHome`), available in both Simplified Chinese ("返回首页") and English ("Go to homepage").
- Provide a visible hover state and a keyboard `:focus-visible` outline using the existing design tokens.

The change MUST NOT alter the logo image dimensions, the brand text typography, or the position of the `VersionBadge`. The collapsed-sidebar layout MUST continue to display only the logo, exactly as before.

#### Scenario: Click brand block from a console page navigates to homepage

- **WHEN** an authenticated user is on any console route (for example `/keys`, `/dashboard`, or `/admin/users`) and clicks the sidebar brand block (logo or site name)
- **THEN** the application navigates via Vue Router to `/` and renders `HomeView` without a full page reload

#### Scenario: Click logo when sidebar is collapsed navigates to homepage

- **WHEN** the sidebar is in the collapsed (icon-only) state and the user clicks the logo image
- **THEN** the application navigates to `/`

#### Scenario: Keyboard activation navigates to homepage

- **WHEN** a keyboard user focuses the brand block link (Tab) and presses Enter or Space
- **THEN** the application navigates to `/`

#### Scenario: Brand block exposes accessible name

- **WHEN** assistive technology inspects the sidebar brand block
- **THEN** the link exposes a localized accessible name equivalent to "Go to homepage" / "返回首页", sourced from the active locale

#### Scenario: Mobile drawer closes after navigation

- **WHEN** the mobile sidebar drawer is open (viewport `< lg` breakpoint) and the user taps the brand block
- **THEN** the application navigates to `/` and the mobile drawer is closed (no visible overlay remains over the homepage)

#### Scenario: Visual layout is preserved

- **WHEN** the sidebar renders in either expanded or collapsed state
- **THEN** the logo size, brand text, and `VersionBadge` appear in the same positions as before the change, with no layout regression

### Requirement: Sidebar Custom Menu Items Support Red-Dot Promotion

`AppSidebar` SHALL delegate the visibility of a per-item red dot on each custom menu entry to the `custom-menu-red-dot` capability. The sidebar itself SHALL:

- Read `custom_menu_red_dot_enabled` and `custom_menu_version` from the cached public settings.
- Instantiate a single `useCustomMenuRedDot` composable at the component level (not per item), so all custom menu items observe the same `shouldShow` value.
- Render the red dot on every entry produced by `customMenuItemsForUser` (and, when applicable, `customMenuItemsForAdmin` if the admin visibility rule is later extended — currently limited to `user`) whenever the composable's `shouldShow` returns `true`.
- Wire `handleMenuItemClick` (used by both desktop sidebar clicks and mobile drawer taps) to invoke `dismiss()` when the clicked path corresponds to a custom menu item.

The red dot SHALL NOT alter the layout or click target of the underlying `router-link`; it SHALL be positioned as a decorative overlay in the top-right of the item's icon or label.

The mobile drawer variant of `AppSidebar` SHALL exhibit identical behavior — red dot rendering and dismissal logic are shared with the desktop sidebar, since both are rendered from the same component instance.

#### Scenario: Custom menu item shows red dot when eligible

- **GIVEN** the public settings response has `custom_menu_red_dot_enabled = true`, a non-empty `custom_menu_version`, and the current user has not dismissed
- **WHEN** `AppSidebar` renders
- **THEN** every user-visible custom menu item shows a red dot

#### Scenario: Clicking a custom menu item dismisses and hides the red dot

- **GIVEN** a red dot visible on a custom menu item
- **WHEN** the user clicks that item
- **THEN** the sidebar navigates as it does today, the dismiss key is written to localStorage, and every custom menu item's red dot disappears within the same tick

#### Scenario: Non-custom sidebar items are unaffected

- **WHEN** `AppSidebar` renders with the feature enabled
- **THEN** built-in navigation items (dashboard, keys, admin, etc.) do NOT show a red dot from this feature; the composable only affects entries produced from `custom_menu_items`

#### Scenario: Mobile drawer parity

- **GIVEN** the viewport is below the `lg` breakpoint and the drawer is open
- **WHEN** the user taps a custom menu item that shows a red dot
- **THEN** the drawer closes (existing behavior), the route changes, and the red dot is dismissed exactly as on desktop
