# Custom Menu Red Dot

## ADDED Requirements

### Requirement: Admin Toggles Custom Menu Red Dot Promotion

The admin SHALL be able to enable or disable a red-dot promotion for the custom menu through the existing `系统设置 → 自定义菜单` view. The setting SHALL be persisted as `custom_menu_red_dot_enabled` (boolean) with a default value of `false`. When `false`, the frontend SHALL NOT render red dots on any custom menu item, regardless of dismissal state.

The admin surface SHALL, adjacent to the toggle, display:

- A short human-readable description explaining that flipping `false → true` or modifying `custom_menu_items` will start a new red-dot cycle.
- The current `custom_menu_version` value (a stable, short hex string) as a read-only label.

#### Scenario: Admin enables the promotion for the first time

- **GIVEN** `custom_menu_red_dot_enabled = false` and one or more items exist
- **WHEN** the admin toggles the switch to `true` and saves
- **THEN** the persisted setting becomes `true` and `custom_menu_version` is recomputed to a value that differs from the previous version

#### Scenario: Admin disables the promotion

- **GIVEN** `custom_menu_red_dot_enabled = true`
- **WHEN** the admin toggles the switch to `false` and saves
- **THEN** the persisted setting becomes `false`, `custom_menu_version` changes, and the frontend stops rendering any custom menu red dots on the next public settings refresh

#### Scenario: Admin performs a no-op save

- **GIVEN** an existing configuration
- **WHEN** the admin submits the settings form without changing `custom_menu_items` or `custom_menu_red_dot_enabled`
- **THEN** `custom_menu_version` is unchanged and users who previously dismissed the current version continue to see no red dots

### Requirement: Backend Derives Custom Menu Version

The system SHALL provide a deterministic pure function `ComputeCustomMenuVersion(itemsJSON string, enabled bool) string` that returns a stable short-hash version token for the current custom menu configuration.

The function SHALL:

- Parse `itemsJSON` into a slice of custom menu items.
- Sort items ascending by `sort_order`.
- For each item, emit only the display-relevant fields (`id`, `label`, `icon_svg`, `url`, `page_slug`, `action`, `visibility`, `sort_order`) with map keys ordered alphabetically.
- Compose the canonical input as `{"enabled":<bool>,"items":<canonical-items-json>}` with no whitespace.
- Compute SHA-256 of the canonical input and return the first 12 hex characters (lowercase).

The function SHALL be pure and side-effect-free; equal canonical inputs SHALL always produce the same output.

#### Scenario: Reordering items in a JSON blob without changing sort_order values

- **GIVEN** two `itemsJSON` inputs `A` and `B` that contain the same items with the same field values but in different array order, and the same `sort_order` values on each item
- **WHEN** `ComputeCustomMenuVersion` is called on both with the same `enabled`
- **THEN** the two outputs are equal

#### Scenario: Changing a label produces a new version

- **GIVEN** two configurations that differ only in one item's `label`
- **WHEN** the version is computed on each
- **THEN** the two outputs are different

#### Scenario: Toggling enabled changes the version

- **GIVEN** identical `itemsJSON`
- **WHEN** the version is computed with `enabled = false` and again with `enabled = true`
- **THEN** the two outputs are different

#### Scenario: Non-display fields do not affect the version

- **GIVEN** two configurations whose items are identical on the display fields but differ in fields not part of the canonical set (e.g. an internal `updated_at`)
- **WHEN** the version is computed on each
- **THEN** the two outputs are equal

### Requirement: Public Settings Endpoint Surfaces Version and Toggle

The public settings response (used by frontends to bootstrap `AppSidebar` and other user-facing surfaces) SHALL include the two fields:

- `custom_menu_red_dot_enabled: boolean`
- `custom_menu_version: string`

Both fields SHALL be present unconditionally. When the admin has not configured the feature, the values SHALL be `false` and the current cached hash respectively (never null / never omitted).

#### Scenario: Feature disabled

- **GIVEN** `custom_menu_red_dot_enabled = false`
- **WHEN** the frontend fetches public settings
- **THEN** the response includes `custom_menu_red_dot_enabled: false` and a non-empty `custom_menu_version` string

#### Scenario: Feature enabled

- **GIVEN** the admin has enabled the feature and saved
- **WHEN** the frontend fetches public settings
- **THEN** the response includes `custom_menu_red_dot_enabled: true` and the same `custom_menu_version` value that the admin sees in the admin panel

### Requirement: User-Facing Red Dot Rendering

Every custom menu item rendered inside `AppSidebar` (both desktop sidebar and mobile drawer) SHALL display a red dot when **all** of the following conditions hold simultaneously:

1. `custom_menu_red_dot_enabled = true`;
2. `custom_menu_version` is a non-empty string;
3. The current user is authenticated (`userId != null`);
4. The user has not previously written the dismiss key `custom-menu-seen:<userId>:<version>` in this browser's `localStorage`.

If any of the above conditions is false, no red dot SHALL be rendered on any custom menu item.

The red dot SHALL be visually consistent with the recharge tab's red dot (small circle in the item's top-right corner). The red dot SHALL be accessible: it SHALL carry an `aria-label` sourced from the localized key `nav.customMenu.newBadgeAria`.

#### Scenario: First-time visitor with feature enabled

- **GIVEN** `custom_menu_red_dot_enabled = true`, `custom_menu_version = "abc123"`, `userId = 42`, and no `custom-menu-seen:42:abc123` key in localStorage
- **WHEN** the user opens any authenticated route that renders `AppSidebar`
- **THEN** every custom menu item shows a red dot

#### Scenario: Admin has feature disabled

- **GIVEN** `custom_menu_red_dot_enabled = false`
- **WHEN** the user opens `AppSidebar`
- **THEN** no custom menu item shows a red dot, regardless of the user's dismissal history

#### Scenario: Anonymous user (no userId)

- **GIVEN** the feature is enabled but the current session has no authenticated user
- **WHEN** the sidebar renders (in whatever context anonymous users see it)
- **THEN** no red dot is shown, and no localStorage read or write occurs

### Requirement: Dismissal Triggers and Persistence

The system SHALL dismiss the custom menu red dot (write the dismiss key) whenever the user takes any of the following actions:

1. Clicks any custom menu item within `AppSidebar` (desktop or mobile drawer).
2. Navigates to a `/custom/:id` route rendered by `CustomPageView` (regardless of whether the entry point was a sidebar click or a direct URL).

Dismissal SHALL:

- Write `localStorage['custom-menu-seen:<userId>:<version>'] = "1"`.
- Take effect immediately in the current tab, causing all custom menu items in `AppSidebar` and any other subscriber to hide their red dots without requiring a reload.
- Propagate to other tabs via the `storage` event within the same browser session.
- Persist until the `custom_menu_version` value changes on the server; a new version restores the red dot on the next public settings refresh.

Dismissal SHALL NOT include a time-based reset (no daily / hourly refresh); the only way to re-show the red dot for a user is to change the version on the server side.

#### Scenario: Sidebar click dismisses all custom menu red dots

- **GIVEN** a user with red dots visible on multiple custom menu items
- **WHEN** the user clicks any one of them
- **THEN** the localStorage key is written, and every custom menu item in the sidebar loses its red dot immediately

#### Scenario: Direct URL to `/custom/:id` dismisses without a sidebar click

- **GIVEN** a user with red dots visible
- **WHEN** the user navigates directly (via typed URL or shared link) to a `/custom/:id` route rendered by `CustomPageView`
- **THEN** the dismiss key is written on component mount, and subsequent visits to any route show no red dots

#### Scenario: Cross-tab dismissal syncs

- **GIVEN** two tabs of the same origin, both signed in as the same user, both showing red dots
- **WHEN** the user dismisses in tab A (by clicking a custom menu item)
- **THEN** tab B's `AppSidebar` red dots disappear on the next event loop tick, without requiring a manual reload

#### Scenario: Admin publishes a new version, red dot returns

- **GIVEN** a user who previously dismissed version `v1`
- **WHEN** the admin saves any change that mints version `v2`, the frontend fetches the updated public settings, and the user opens `AppSidebar`
- **THEN** the red dots reappear on all custom menu items until the user dismisses again for `v2`

#### Scenario: Different user on shared browser

- **GIVEN** user A dismissed version `v1`
- **WHEN** user B signs in on the same browser and opens `AppSidebar` while `custom_menu_version` is still `v1`
- **THEN** user B sees the red dots because the dismiss key includes `<userId>` and A's key does not match B
