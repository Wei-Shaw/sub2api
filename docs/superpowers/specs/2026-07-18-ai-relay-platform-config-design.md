# AI Relay Platform Configuration Design

## Goal

Replace the Agnes-specific AI Relay plugin page with a generic platform
configuration page. Administrators create multiple named relay configurations
for adapters that are actually registered by the plugin. The first registered
adapter remains Agnes Image; OpenCode appears only after its adapter is
implemented and registered.

## User Experience

The plugin page follows the main site's account-management layout without
embedding or importing the main frontend:

- A compact toolbar has search, platform filter, enabled-state filter, refresh,
  and an add-configuration action.
- A dense table shows configuration name, platform, target URL, default model,
  status, stable slug, and row actions.
- Create and edit use one modal form. A platform selector lists only adapters
  returned by the plugin backend.
- The form shows fields declared by the selected platform: target URL, default
  model, model map, quality map, maximum output count, and enabled state for
  Agnes Image.
- Each row exposes the exact main-site account URL:
  `http://plugin-server:8091/plugins/ai-relay/<platform>/<slug>`.

Multiple configurations per platform are supported. Names are administrator
labels; slugs are stable lowercase route identifiers and must remain unique per
platform.

## Backend Contract

Add a platform-descriptor registry beside the existing adapter registry. A
descriptor has a key, display name, supported operation, and field definition.
The admin API exposes only descriptors that have a registered adapter.

Extend the route record with a required non-secret `name`. Existing database
rows are migrated with their slug as the initial name. Existing Agnes routes
remain routable without changing their slug, target URL, or default model.

The API becomes:

- `GET /plugins/ai-relay/api/platforms`
- `GET /plugins/ai-relay/api/routes?platform=&search=&enabled=`
- `POST /plugins/ai-relay/api/routes`
- `PUT /plugins/ai-relay/api/routes/{platform}/{slug}`
- `DELETE /plugins/ai-relay/api/routes/{platform}/{slug}`

Create and update reject an adapter key that is not registered. Public relay
behavior and its credential boundary stay unchanged: the main-site account
Bearer key is forwarded to the resolved target but never stored.

## Architecture

Keep the plugin directory split by responsibility:

```text
plugins/ai-relay/
  backend/
    adapter.go       # adapter and platform-descriptor registries
    config.go        # persistent route model and validation
    handler.go       # public relay plus admin APIs
    agnes.go         # Agnes Image adapter and descriptor
  web/
    assets/app.js    # platform configuration page behavior
    assets/app.css   # account-management visual language
```

Future platforms implement an image adapter and descriptor in their own file
or package, then register themselves. They do not add branches to the page's
route handling or mutate existing platform configuration.

## Error Handling And Testing

- The page shows API errors next to the form or table action that caused them.
- Disabled routes remain editable but cannot be used by public relay traffic.
- Deleting a route requires an in-page confirmation dialog.
- Tests cover registered-platform listing, rejecting unregistered platforms,
  name migration/defaulting, list filtering, multiple Agnes configurations,
  and the existing public relay contract.

## Non-Goals

- Adding an OpenCode relay adapter in this change.
- Storing provider API keys in the plugin.
- Reusing or mounting main-site Vue components across the plugin boundary.
