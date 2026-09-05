# AI Relay Sticky Header Layout Design

## Scope

Update only the standalone AI Relay plugin layout. Do not modify main-site frontend files.

## Desktop Layout

- Keep the page as a full-height vertical flex container.
- Keep header, filters, alerts, and selection controls non-shrinking.
- Let the table wrapper fill the remaining content height and own vertical/horizontal scrolling.
- Make the table header sticky at the top of the table wrapper with an opaque background and elevated stacking order.
- Keep pagination non-shrinking at the absolute bottom of the content area, with no extra top gap beyond its border/padding band.

The table header uses container-relative sticky positioning, never viewport-fixed positioning, so it does not overlap Toast notifications or the page header.

## Mobile Layout

Use the same sticky header inside the table's scroll container on mobile. The shell continues to use natural document height and the table keeps horizontal overflow for the complete Plugin URL column.

## Verification

- CSS contract tests cover sticky table header, opaque header background, stacking order, and bottom pagination spacing.
- Existing route, pagination, Toast, and API tests remain passing.
- Frontend typecheck and production build pass.
- No main-site frontend files are modified.
