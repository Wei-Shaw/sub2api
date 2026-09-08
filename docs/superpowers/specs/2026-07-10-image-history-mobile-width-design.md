# Image History Mobile Width Design

## Goal

Reduce the image-generation history drawer width on phone-sized screens so the underlying page remains visibly recognizable while the history list stays usable.

## Responsive Behavior

- At viewport widths up to 767px, set the collapsed history drawer width to `min(76vw, 280px)`.
- From 768px through 1023px, retain the existing `min(88vw, 320px)` drawer width.
- At desktop widths, retain the existing inline history sidebar layout.

## Scope

The change is limited to the mobile media rule in `plugin-service/plugins/image-generation/web/index.html`. Drawer transitions, overlay behavior, history content spacing, and all other image-generation controls remain unchanged.

## Verification

- Add a frontend source assertion covering the `max-width: 767px` media rule and the selected width value.
- Run the image-generation plugin package tests.
- Inspect the final diff to confirm that no generated assets or unrelated UI rules changed.
