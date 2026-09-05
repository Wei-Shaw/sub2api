# Image Generation Compact Select Width Design

## Goal

Make the image generation composer's model, size, and output-count selects use only the width needed by their currently selected text, while preserving a small amount of horizontal padding for readability and the native dropdown indicator.

## Scope

- Change only the three selects in `PromptComposer.vue`: model, size, and output count.
- Keep their current height, pill shape, typography, native dropdown behavior, keyboard behavior, and emitted update events.
- Do not change confirmation dialogs, image preview controls, the send/stop button, reference-image controls, or any other buttons and selects.

## Design

Wrap each target select in a shared, local sizing container. The container includes an `aria-hidden` text mirror that renders the current visible option text with matching typography. The select remains positioned over that mirror and retains its native interaction behavior.

Vue-derived display strings provide the mirror content:

- Model: the current model value.
- Size: the visible size label, such as `1024 × 1024`.
- Output count: the current value followed by `张`.

The mirror determines the content width. CSS adds only the existing left text padding and enough right padding for the native dropdown indicator. When the selected value changes, Vue updates the mirror in the same render cycle, so the control width follows the selected text without manual pixel measurement.

On narrow screens, the existing composer wrapping remains unchanged. A maximum width prevents an unusually long model name from overflowing the composer; overflowing selected text is clipped within that control only.

## Accessibility

The native `select` elements and their visually hidden labels remain the accessible controls. Mirror text is hidden from assistive technology and cannot receive pointer or keyboard input.

## Verification

- Component tests verify all three sizing mirrors render their current visible values.
- Component tests update model, size, and output count and confirm the existing update events still emit the correct values.
- Build and type checking must pass.
- Browser verification checks desktop and mobile layouts, confirms each control follows its selected text, and confirms unrelated controls are visually unchanged.
