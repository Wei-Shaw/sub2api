# Console Navigation

## ADDED Requirements

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
