# AI Relay Pagination Layout Design

## Scope

Update only the standalone AI Relay plugin frontend layout. Do not modify the main-site pagination component or main-site view code.

## Desktop Layout

The plugin shell uses a full-height vertical flex layout:

1. Header, filters, alerts, and selection controls remain flex-shrink-0.
2. The table wrapper fills remaining space with `flex: 1` and `min-height: 0`.
3. The table wrapper owns overflow scrolling so long route lists do not push pagination off screen.
4. Pagination remains flex-shrink-0 at the bottom of the content area with a top border and white background matching the main-site table pagination surface.

The pagination is not `position: fixed` and never overlays table rows or Toast notifications.

## Mobile Layout

At the existing mobile breakpoint, the shell returns to natural document height, the table wrapper uses content height, and the pagination follows the table normally. Horizontal table scrolling remains available when the complete Plugin URL column exceeds the viewport.

## Behavior

Keep the existing previous/next controls, page indicator, page-size selector, and page loading behavior unchanged. This change only alters layout and responsive overflow behavior.

## Verification

- Component/style tests verify the shell/table/pagination classes and existing pagination behavior remains intact.
- Frontend typecheck and production build pass.
- Desktop and mobile rendered checks confirm pagination is visible at the bottom without covering table content.
- No main-site frontend files are modified.
