## MODIFIED Requirements

### Requirement: Admin Configures Per-Item Red Dot Reminders

The admin SHALL be able to enable or disable a red-dot reminder **per custom menu item** through the existing `系统设置 → 自定义菜单` view. Each `CustomMenuItem` SHALL carry a boolean `show_red_dot` (default `false`), which controls whether that specific item participates in red-dot rendering. There is no longer a global on/off switch for the whole custom menu.

The admin surface SHALL:

- Render a Toggle inside each menu item's form (adjacent to the "Documentation URL" field), bound to `show_red_dot`.
- Display the current `custom_menu_version` value once at the top of the custom-menu card as a read-only label, so admins can verify their save reached users.

Toggling `show_red_dot` on a single item SHALL NOT by itself mint a new `custom_menu_version`; it only affects whether that item is eligible to render a red dot. To force a red dot to re-appear for users who already dismissed it, the admin MUST modify one of the display fields (`label`, `url`, `icon_svg`, `page_slug`, `action`, `visibility`, or `sort_order`).

#### Scenario: Admin enables the reminder for a single item

- **GIVEN** two items `A` and `B` with `show_red_dot = false`
- **WHEN** the admin toggles `A.show_red_dot = true` and saves without changing anything else
- **THEN** the response persists `A.show_red_dot = true`, `B.show_red_dot = false`, `custom_menu_version` is unchanged, and only item `A` is eligible for a red dot on the frontend

#### Scenario: Admin performs a display-field change

- **GIVEN** an existing configuration
- **WHEN** the admin changes any item's `label`, `url`, `icon_svg`, `sort_order`, `visibility`, `action`, or `page_slug`
- **THEN** `custom_menu_version` is recomputed to a new value; users who dismissed the previous version see red dots again on items where `show_red_dot = true`

#### Scenario: Admin performs a no-op save

- **GIVEN** an existing configuration
- **WHEN** the admin submits the settings form without changing any display field
- **THEN** `custom_menu_version` is unchanged and users who previously dismissed the current version continue to see no red dots

### Requirement: Backend Derives Custom Menu Version

The system SHALL provide a deterministic pure function `ComputeCustomMenuVersion(itemsJSON string) string` that returns a stable short-hash version token for the current custom menu display configuration.

The function SHALL:

- Parse `itemsJSON` into a slice of custom menu items.
- Sort items ascending by `sort_order`; ties broken by `id` alphabetically.
- For each item, emit only the display-relevant fields (`id`, `label`, `icon_svg`, `url`, `page_slug`, `action`, `visibility`, `sort_order`) with map keys ordered alphabetically. Non-display fields, notably `show_red_dot` and `doc_url`, SHALL NOT be included in the canonical input.
- Compose the canonical input as `{"items":<canonical-items-json>}` with no whitespace.
- Compute SHA-256 of the canonical input and return the first 12 hex characters (lowercase).

The function SHALL be pure and side-effect-free; equal canonical inputs SHALL always produce the same output.

#### Scenario: Reordering items in a JSON blob without changing sort_order values

- **GIVEN** two `itemsJSON` inputs `A` and `B` that contain the same items with the same field values but in different array order, and the same `sort_order` values on each item
- **WHEN** `ComputeCustomMenuVersion` is called on both
- **THEN** the two outputs are equal

#### Scenario: Changing a label produces a new version

- **GIVEN** two configurations that differ only in one item's `label`
- **WHEN** the version is computed on each
- **THEN** the two outputs are different

#### Scenario: Toggling show_red_dot does NOT change the version

- **GIVEN** two configurations that differ only in one item's `show_red_dot` flag
- **WHEN** the version is computed on each
- **THEN** the two outputs are equal

#### Scenario: Non-display fields do not affect the version

- **GIVEN** two configurations whose items are identical on the display fields but differ in fields not part of the canonical set (e.g. an internal `updated_at` or `doc_url`)
- **WHEN** the version is computed on each
- **THEN** the two outputs are equal

### Requirement: Public Settings Endpoint Surfaces Version and Per-Item Flag

The public settings response (used by frontends to bootstrap `AppSidebar` and other user-facing surfaces) SHALL include:

- `custom_menu_version: string` — always present, current cached hash (never null / never omitted).
- `custom_menu_items: CustomMenuItem[]` — each entry SHALL include `show_red_dot: boolean` (defaulting to `false` when the admin has never set it).

The response SHALL NOT include a top-level `custom_menu_red_dot_enabled` field; that field has been removed.

#### Scenario: All items opted out

- **GIVEN** every item has `show_red_dot = false`
- **WHEN** the frontend fetches public settings
- **THEN** the response includes a non-empty `custom_menu_version` and every item's `show_red_dot = false`

#### Scenario: One item opted in

- **GIVEN** the admin has set `show_red_dot = true` on exactly one item and saved
- **WHEN** the frontend fetches public settings
- **THEN** that item's `show_red_dot = true` in the response and other items remain `false`; `custom_menu_version` matches the value the admin sees in the admin panel

### Requirement: User-Facing Red Dot Rendering (Per Item)

Every custom menu item rendered inside `AppSidebar` (both desktop sidebar and mobile drawer) SHALL display a red dot when **all** of the following conditions hold simultaneously **for that item**:

1. `item.show_red_dot === true`;
2. `custom_menu_version` is a non-empty string;
3. The current user is authenticated (`userId != null`);
4. The user has not previously written the dismiss key `custom-menu-seen:<userId>:<itemId>:<version>` in this browser's `localStorage`.

If any of the above conditions is false for a given item, no red dot SHALL be rendered on that item. Items with `show_red_dot = false` SHALL never render a red dot regardless of dismiss history.

The red dot SHALL be visually consistent with the recharge tab's red dot (small circle in the item's top-right corner). The red dot SHALL be accessible: it SHALL carry an `aria-label` sourced from the localized key `nav.customMenu.newBadgeAria`.

#### Scenario: One item opted in, others not

- **GIVEN** three items with `show_red_dot = [true, false, true]`, `custom_menu_version = "abc123"`, `userId = 42`, and no `custom-menu-seen:42:*:abc123` keys in localStorage
- **WHEN** the user opens any authenticated route that renders `AppSidebar`
- **THEN** items 1 and 3 show red dots; item 2 shows no red dot

#### Scenario: Anonymous user (no userId)

- **GIVEN** any items with `show_red_dot = true`
- **WHEN** the sidebar renders in an anonymous session
- **THEN** no red dot is shown, and no localStorage read or write occurs

### Requirement: Dismissal Triggers and Persistence (Per Item)

The system SHALL dismiss a specific item's red dot (write the item's dismiss key) whenever the user takes any of the following actions:

1. Clicks that specific custom menu item within `AppSidebar` (desktop or mobile drawer). Only the clicked item's red dot is cleared; other items keep their red dots.
2. Navigates to `/custom/:id` rendered by `CustomPageView`. On mount, the component derives the `itemId` from the route and dismisses that item's red dot only.

Dismissal SHALL:

- Write `localStorage['custom-menu-seen:<userId>:<itemId>:<version>'] = "1"`.
- Take effect immediately in the current tab, causing that specific item's red dot to disappear across all subscribers (sidebar, drawer, page view) without requiring a reload.
- Propagate to other tabs via the `storage` event within the same browser session.
- Persist until the `custom_menu_version` value changes on the server (i.e. admin edits any display field of any item); a new version restores red dots on all items still opted in via `show_red_dot`.

Dismissal SHALL NOT include a time-based reset (no daily / hourly refresh); the only way to re-show a red dot for a user is to change the version on the server side or for the admin to keep the item's `show_red_dot = true` and later mint a new version via any display-field edit.

#### Scenario: Sidebar click dismisses only the clicked item

- **GIVEN** three items all showing red dots
- **WHEN** the user clicks the second item
- **THEN** only the second item's dismiss key is written; items 1 and 3 continue to show red dots on the same page and after refresh

#### Scenario: Direct URL to `/custom/:id` dismisses that item only

- **GIVEN** three items all showing red dots
- **WHEN** the user opens `/custom/<id-of-item-3>` directly (typed URL or shared link)
- **THEN** only item 3's dismiss key is written on `CustomPageView` mount; items 1 and 2 continue to show red dots

#### Scenario: Cross-tab dismissal syncs per item

- **GIVEN** two tabs of the same origin, both signed in as the same user, both showing red dots on the same three items
- **WHEN** the user dismisses item 2 in tab A
- **THEN** tab B's item 2 red dot disappears on the next event loop tick; items 1 and 3 in tab B remain

#### Scenario: Admin publishes a new version, red dots return only for opted-in items

- **GIVEN** a user who previously dismissed items 1 and 3 for version `v1`, item 2 was `show_red_dot = false`
- **WHEN** the admin edits any display field which mints version `v2` and the frontend refetches public settings
- **THEN** items 1 and 3 show red dots again (until dismissed for `v2`); item 2 shows no red dot as long as its `show_red_dot` remains false

#### Scenario: Different user on shared browser

- **GIVEN** user A dismissed items 1 and 3 for version `v1`
- **WHEN** user B signs in on the same browser and opens `AppSidebar` while `custom_menu_version` is still `v1`
- **THEN** user B sees red dots on items 1 and 3 because the dismiss key includes `<userId>` and A's key does not match B
