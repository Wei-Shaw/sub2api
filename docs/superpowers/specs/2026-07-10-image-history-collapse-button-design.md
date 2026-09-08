# Image History Collapse Button Design

## Goal

Make the image-generation history sidebar controls easier to understand by placing the collapse action before the sidebar title and ensuring each arrow points toward the resulting movement.

## Layout And Direction

- In the expanded history sidebar, place the collapse button at the left of the `History` title.
- The expanded-state collapse button displays a left-pointing chevron (`<`) to indicate that the sidebar will retract to the left.
- When the sidebar is collapsed, the fixed pull-out handle displays a right-pointing chevron (`>`) to indicate that the sidebar will expand to the right.
- Hover and focus states do not change either chevron direction.

## Button Style

- Use a 32px square button with an 8px corner radius.
- Use a white surface, neutral gray border, and neutral foreground in light mode.
- On hover, use a light gray surface, a stronger border, and a small shadow.
- On active press, apply only a subtle scale reduction.
- On keyboard focus, show a visible focus ring.
- Provide equivalent neutral contrast in dark mode.

## Scope

Modify only the injected history drawer chrome and its styles in `plugin-service/plugins/image-generation/web/index.html`. Preserve drawer state logic, overlay behavior, responsive width rules, and the placement of the collapsed pull-out handle.

## Verification

- Add frontend source assertions for button-before-title DOM order, the compact outlined style, and opposite expanded/collapsed chevron directions.
- Run the image-generation plugin package tests.
- Inspect the diff to confirm no bundled asset or unrelated UI changes.
