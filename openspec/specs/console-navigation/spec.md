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
