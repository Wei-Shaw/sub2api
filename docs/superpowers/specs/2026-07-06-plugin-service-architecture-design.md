# Plugin Service Multi-Plugin Architecture Design

Date: 2026-07-06

## Overview

This design refactors `plugin-service` from a single-plugin prototype into a reusable plugin host. The immediate goal is to remove inline frontend code from Go handlers and establish clear boundaries so future plugins can be added without expanding `internal/handler/app.go` into a larger monolith.

The first plugin to migrate is the existing image-generation workspace. Its current behavior, authentication model, and history visibility rules must remain intact during the refactor.

## Goals

- Keep `plugin-service` independently deployable.
- Preserve the current shared-auth flow with the main service.
- Remove frontend HTML, CSS, and JavaScript from Go source files.
- Support multiple plugins with a predictable folder and routing structure.
- Default to a single host service and a single public base URL.
- Preserve the option for a plugin to use a separately deployed frontend later.
- Keep admin and user history visibility rules unchanged.
- Avoid mixing architecture refactor with unrelated data-storage changes.

## Non-Goals

- Replacing the current authentication model.
- Changing image-generation provider behavior.
- Migrating history storage from in-memory to SQL in the same change.
- Introducing a separate frontend deployment pipeline for every plugin by default.
- Building a generic marketplace or plugin installation system.

## Current Problems

The current implementation has four structural problems:

1. `plugin-service/internal/handler/app.go` mixes host concerns, page rendering, authentication endpoints, and image-generation business endpoints in one file.
2. The image-generation frontend is embedded as a large HTML string inside Go code, making UI changes hard to review and reuse.
3. Routing assumes one implicit plugin, so paths such as `/app` and `/api/generate` are tightly coupled to the current image-generation workspace.
4. Adding a second plugin would require editing core host files instead of adding a mostly self-contained module.

## Decision Summary

Adopt a host-and-plugin architecture with the following properties:

- `plugin-service` becomes the host runtime.
- Each plugin gets a standard directory template: `manifest`, `backend`, and `web`.
- The host serves plugin frontend assets by default.
- The host can later point a plugin to a remote frontend entry URL if the plugin needs independent deployment.
- Plugin APIs are namespaced by plugin key.
- Shared concerns such as session validation, principal resolution, and common response helpers stay in host code.

## Architecture

### 1. Host Responsibilities

The host is responsible for:

- Verifying launch tickets.
- Creating and validating the `plugin_session` cookie.
- Resolving the current principal.
- Applying common security headers.
- Registering plugins and exposing plugin metadata.
- Serving plugin entry pages and plugin static assets.
- Mounting plugin-specific API routes.
- Providing shared infrastructure for history access, role filtering, and future shared persistence abstractions.

The host is not responsible for:

- Containing plugin-specific HTML or JavaScript inline.
- Owning plugin-specific API details.
- Knowing business-specific request fields beyond shared infrastructure concerns.

### 2. Plugin Responsibilities

Each plugin owns:

- Its own manifest metadata.
- Its own backend handlers and services.
- Its own frontend source and build artifacts.
- Its own page composition and API usage.

The first migrated plugin is the image-generation plugin.

### 3. Frontend Hosting Modes

Two hosting modes are supported:

1. Hosted mode, which is the default.
   The plugin frontend build output lives under the plugin directory and is served by `plugin-service`.

2. Remote mode, which is optional for future use.
   The plugin manifest may provide a remote entry URL. In that case the main service or host plugin index can embed that external page while keeping the same launch-ticket and plugin-session model.

Hosted mode is the default path for all new plugins because it is the simplest operational model and keeps everything behind one public plugin-service URL.

## Proposed Directory Structure

```text
plugin-service/
  cmd/
    server/
  internal/
    config/
    host/
      auth/
      handler/
      httpx/
      principal/
      server/
    pluginregistry/
    shared/
      history/
      session/
  plugins/
    image-generation/
      manifest/
        plugin.go
      backend/
        handlers.go
        service.go
      web/
        src/
        dist/
  migrations/
  README.md
```

### Structure Notes

- `internal/host/...` contains only host concerns.
- `internal/pluginregistry` owns plugin registration and lookup.
- `plugins/<key>/manifest` defines the plugin key, labels, entry mode, and mount details.
- `plugins/<key>/backend` contains plugin-specific HTTP handlers and business services.
- `plugins/<key>/web/src` holds editable frontend source.
- `plugins/<key>/web/dist` holds host-served build output.

## Plugin Manifest Contract

Each plugin manifest must define enough metadata for the host to mount and expose the plugin consistently.

Required fields:

- `key`
- `name`
- `description`
- `enabled`
- `default_entry_path`
- `frontend_mode`

Optional fields:

- `remote_entry_url`
- `icon`
- `version`

`frontend_mode` values:

- `hosted`
- `remote`

Manifest rules:

- `key` must be unique across all registered plugins.
- `default_entry_path` is used when the host redirects to the plugin page after launch.
- `remote_entry_url` is only valid when `frontend_mode=remote`.

## Routing Design

### Host Routes

- `GET /healthz`
- `GET /launch?ticket=...&plugin=<key>&path=...`
- `GET /dev/login?...&plugin=<key>&path=...`
- `GET /api/me`
- `GET /api/plugins`
- `GET /api/plugins/{key}`

### Plugin Page and Asset Routes

- `GET /plugins/{key}`
- `GET /plugins/{key}/assets/...`

### Plugin API Routes

- `POST /api/plugins/{key}/...`
- `GET /api/plugins/{key}/...`

### Backward Compatibility Strategy

The current image-generation paths are:

- `GET /app`
- `GET /api/config`
- `POST /api/generate`
- `GET /api/creations`
- `GET /api/history`
- `GET /api/history/{id}`
- `POST /api/history/{id}/retry`
- `POST /api/history/{id}/cancel`

During the first migration, these legacy routes may be kept as thin compatibility aliases that internally forward to the image-generation plugin routes. This keeps existing menu configuration stable while allowing the internal architecture to move to namespaced plugin routing.

Target namespaced routes for image-generation:

- `GET /plugins/image-generation`
- `GET /api/plugins/image-generation/config`
- `POST /api/plugins/image-generation/generate`
- `GET /api/plugins/image-generation/creations`
- `GET /api/plugins/image-generation/history`
- `GET /api/plugins/image-generation/history/{id}`
- `POST /api/plugins/image-generation/history/{id}/retry`
- `POST /api/plugins/image-generation/history/{id}/cancel`

## Authentication and Session Model

The current authentication model remains unchanged:

1. The main service issues a short-lived launch ticket.
2. The plugin host verifies the ticket.
3. The plugin host creates the `plugin_session` httpOnly cookie.
4. Plugin APIs rely on the plugin session instead of the main-service bearer token.

Role visibility rules remain unchanged:

- Admin users can read all plugin history records.
- Normal users can only read records for their own `user_id`.

The principal should continue to carry the selected plugin key so that plugin-specific APIs and future audit trails remain scoped correctly.

## History and Shared Data Boundaries

History remains a shared host capability, not a per-plugin reinvention. The host keeps the rule enforcement and shared repository abstraction, while plugins consume it through interfaces.

Shared requirements:

- Every history record must include `plugin_key`.
- Role-based filtering remains centralized.
- Sensitive request fields such as `provider_api_key` remain sanitized in API responses.
- Creation-list behavior for image-generation remains available.

This refactor does not require changing the current in-memory storage implementation. A later change may swap the repository implementation for SQL while preserving the same service boundaries.

## Frontend Design Rules

Frontend code must move out of Go source files.

Rules for plugin frontend development:

- No large inline HTML or JavaScript strings in handlers.
- Each plugin frontend lives under `plugins/<key>/web/`.
- The host serves built assets from `web/dist` in hosted mode.
- The frontend calls plugin-scoped API routes instead of root-level single-plugin routes.
- Shared host session behavior remains cookie-based, so no new token injection is required in the frontend.

For the image-generation plugin, the existing page can initially be moved as plain static HTML, CSS, and JavaScript into `web/` without introducing a framework. That keeps the migration focused on architecture rather than adding a frontend stack migration at the same time.

## Registry Model

The host needs a plugin registry so adding a plugin does not require editing many unrelated files.

The registry is responsible for:

- Registering plugins at startup.
- Validating duplicate keys.
- Exposing metadata for `/api/plugins`.
- Providing route mounts for page handlers, static assets, and API handlers.

Expected outcome for adding a new plugin:

1. Copy the plugin template directory.
2. Update the manifest.
3. Implement plugin backend handlers.
4. Add frontend files under `web/`.
5. Register the plugin once in the registry bootstrap.

This keeps extension work mostly additive instead of invasive.

## Migration Plan

### Phase 1. Host Extraction

- Move shared auth, session, principal, and common response helpers out of `internal/handler/app.go`.
- Introduce host-oriented packages under `internal/host`.
- Keep behavior unchanged.

### Phase 2. Registry Introduction

- Add a plugin registry package.
- Register the image-generation plugin as the first plugin module.
- Expose `/api/plugins` and `/api/plugins/{key}`.

### Phase 3. Frontend Extraction

- Move the image-generation UI out of Go string literals into `plugins/image-generation/web/`.
- Serve the page and assets through host static routing.
- Preserve the same user-visible page behavior.

### Phase 4. Plugin API Namespacing

- Move image-generation handlers into `plugins/image-generation/backend`.
- Mount namespaced plugin routes.
- Keep legacy aliases temporarily for backward compatibility.

### Phase 5. Cleanup

- Remove obsolete single-plugin assumptions from host code.
- Reduce or eliminate the original monolithic `app.go`.
- Update documentation for plugin creation and route usage.

## Testing Strategy

The refactor must preserve behavior while changing boundaries. Testing should focus on contracts rather than file layout.

Required coverage:

- Launch ticket verification still creates a plugin session.
- Dev login still works when enabled.
- `/api/plugins` returns registered plugin metadata.
- Hosted plugin page routes render successfully.
- Plugin static assets are served correctly.
- Image-generation plugin APIs behave the same as before.
- Role-aware history visibility remains correct.
- Legacy route aliases continue to work during migration.

## Risks and Mitigations

### Risk 1. Route Breakage

If existing menu configuration still points at `/app`, users may lose access after the refactor.

Mitigation:

- Keep compatibility aliases during the migration.
- Only remove them after the main-service menu configuration is updated.

### Risk 2. Host and Plugin Responsibilities Blur Again

If plugin-specific logic leaks back into host packages, the refactor will recreate the same problem with different folders.

Mitigation:

- Keep plugin-specific handlers in `plugins/<key>/backend`.
- Keep host packages focused on shared infrastructure only.

### Risk 3. Frontend Build Complexity Grows Too Early

Adding a framework or multi-bundle toolchain now could distract from the architecture goal.

Mitigation:

- Start by moving the current frontend into plain hosted assets.
- Introduce a richer frontend toolchain later only if a plugin needs it.

### Risk 4. Storage Refactor Expands Scope

Trying to move both architecture and persistence at once will increase risk.

Mitigation:

- Keep the existing repository behavior for this refactor.
- Make persistence replacement a later isolated change.

## Implementation Guidance

The first implementation pass should optimize for separation of concerns, not for ambitious feature growth.

Priority order:

1. Establish host and registry boundaries.
2. Extract the image-generation plugin frontend from Go.
3. Move image-generation backend routes behind plugin namespacing.
4. Preserve and test legacy compatibility routes.
5. Leave storage replacement for a separate follow-up.

## Expected Result

After this refactor:

- `plugin-service` becomes a stable plugin host rather than a single-plugin prototype.
- The image-generation workspace becomes the first real plugin module.
- Future plugins can be added through a predictable template.
- Frontend code is no longer embedded in Go handlers.
- The system keeps one default deployment URL while still supporting a future independently deployed plugin frontend when needed.
