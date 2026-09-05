# AI Relay Copy Toast Design

## Scope

Add copy-result feedback to the standalone AI Relay plugin frontend. Do not import main-site components or modify main-site frontend files.

## Component

Create a small plugin-local Toast component that follows the main site's notification appearance:

- fixed at the top-right of the viewport;
- white card with a subtle border and shadow;
- green left border and status icon for success;
- red left border and status icon for errors;
- message content and an accessible close button;
- `aria-live="polite"` and `aria-atomic="true"` on the notification container.

Only one copy-result toast is required at a time. A new copy attempt replaces the current toast.

## Behavior

After the Plugin URL is copied successfully, show `Plugin URL copied` for 3000 milliseconds.

When the Clipboard API is unavailable or rejects the write, show `Failed to copy Plugin URL` for 5000 milliseconds.

Users can close either toast immediately. Component timers are cleared when a toast is replaced, dismissed, or the component is unmounted.

## Integration

`App.vue` owns the current toast state and calls the existing `copyRouteURL` action. The Toast component only renders state and emits a dismiss event; it does not access the Clipboard API or route data.

## Verification

- A component test proves successful copy renders the success toast.
- A component test proves rejected copy renders the error toast.
- Toast tests cover manual dismissal and automatic dismissal durations.
- Plugin frontend tests, typecheck, and production build pass.
- Git status confirms no main-site frontend files changed.
