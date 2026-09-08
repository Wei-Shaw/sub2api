# AI Relay Slug Rename Design

## Goal

Allow administrators to change an AI Relay route Slug from the plugin service page.

## Behavior

- Platform remains immutable while editing.
- Slug becomes editable and uses the existing lowercase/pattern validation.
- If the edited Slug is unchanged, use the existing update request.
- If the edited Slug changes, delete the old `{platform, slug}` route first, then create the submitted route under the new Slug.
- After a successful save, the list and Plugin URL use the new Slug; the old URL immediately returns not found.
- If creation fails after deletion, show the creation error and leave the old route deleted, matching the selected non-atomic migration strategy.

## Scope

- Modify only the standalone AI Relay plugin frontend and its tests.
- Reuse existing `deleteRoutes` and `createRoute` APIs; no main-site changes and no database schema changes.

## Testing

- Verify unchanged Slug still calls `updateRoute`.
- Verify changed Slug calls `deleteRoutes` with the old key, then `createRoute` with the new key and preserved mapping data.
- Verify the Slug input is enabled during edit and remains disabled only for Platform.
- Run all plugin frontend tests, typecheck, and build.
