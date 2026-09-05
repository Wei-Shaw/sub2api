# AI Relay Path Mappings Design

## Goal

Add route-level upstream path mappings to the AI Relay plugin. A configured mapping replaces the path portion of an upstream request while preserving the route's `base_url` scheme and host. Requests without a matching mapping keep the existing behavior.

This change also exposes transparent relay endpoints for OpenAI Responses requests:

- `/plugins/ai-relay/{platform}/{slug}/v1/responses`
- `/plugins/ai-relay/{platform}/{slug}/v1/responses/compact`

The feature performs URL routing only. It does not convert request bodies, response bodies, SSE events, or API protocols.

## Configuration Contract

Each `RouteConfig` gains a `path_mappings` object:

```json
{
  "responses/compact": "api/paas/v4/chat/completions",
  "models": "api/paas/v4/models"
}
```

Both source and target are relative paths. Normalization trims whitespace and leading or trailing slashes. Empty entries are discarded. Full target URLs and targets containing a scheme or host are rejected.

Source matching is exact after normalization. To preserve the reference project's operator ergonomics, these source forms address the same inbound endpoint:

- `responses/compact`
- `/responses/compact`
- `v1/responses/compact`
- `/v1/responses/compact`

Stored mappings use normalized path strings without leading or trailing slashes. If equivalent source forms appear more than once after normalization, the configuration is invalid rather than depending on map iteration order.

## URL Resolution

All AI Relay upstream requests pass through one path resolver. The resolver receives the route configuration and a canonical endpoint path such as `models`, `chat/completions`, `images/generations`, `images/edits`, `responses`, or `responses/compact`.

When a mapping matches, the resolver preserves only the `base_url` scheme, authority, and any user information allowed by existing URL validation, then replaces its complete path with the mapped target. For example:

```text
base_url:              https://open.bigmodel.cn/v1
responses/compact ->   api/paas/v4/chat/completions
resolved URL:          https://open.bigmodel.cn/api/paas/v4/chat/completions
```

When no mapping matches, the resolver appends the canonical endpoint to the complete configured base URL, preserving current behavior:

```text
base_url:              https://open.bigmodel.cn/v1
canonical endpoint:    responses/compact
resolved URL:          https://open.bigmodel.cn/v1/responses/compact
```

Incoming query parameters are forwarded unchanged for transparent proxy endpoints. A mapped target may not embed a query or fragment; this keeps path configuration separate from per-request parameters and prevents ambiguous merging.

## Proxy Behavior

Existing image generation retains its adapter request and response conversion. Only its final upstream URL is resolved through the shared mapping logic.

Models, Chat Completions, Responses, and Responses Compact use transparent proxying:

- preserve the incoming HTTP method;
- stream the request body without JSON interpretation;
- forward `Authorization`, `Content-Type`, `Accept`, and other end-to-end request headers;
- omit hop-by-hop headers;
- preserve upstream status, end-to-end response headers, and streaming response body;
- use the existing account proxy selection via `X-Sub2api-Proxy-Id`;
- retain the existing authentication and route lookup errors.

The image edit endpoint remains multipart-aware because the Agnes adapter converts edits into its generation payload. Its mapped canonical path is `images/generations`, matching the actual upstream operation performed by the adapter.

## Persistence And API

`plugin_ai_relay_routes` gains a non-null `path_mappings JSONB` column with an empty object default. Runtime schema initialization adds the column with `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`, so existing installations require no manual migration.

Create, update, get, and list operations include `path_mappings`. An omitted or empty mapping produces `{}`. The in-memory repository and SQL repository return defensive copies so callers cannot mutate persisted route state through map references.

Unknown JSON fields remain rejected by the route API. Invalid path mappings return the existing `400 invalid relay route configuration` response without partially saving a route.

## Frontend Ownership

The AI Relay management application belongs entirely to the plugin service. The main-site frontend only exposes its existing custom-menu configuration; operators add a menu item whose URL points to `http://plugin-server:8091/plugins/ai-relay`.

The plugin owns a standalone Vue/Vite application under `plugin-service/plugins/ai-relay/frontend`. Its build output is generated into `plugin-service/plugins/ai-relay/web` and served by the existing `frontendhost.RegisterHostedPlugin` integration at `/plugins/ai-relay`. The hosted frontend calls same-origin `/plugins/ai-relay/api/*` endpoints, allowing the existing frontend auth bridge to attach the current session token.

The current redirect-only plugin HTML is replaced by the built application. No main-site component, router entry, style, API client, or test is added or changed for this feature. The two main-site files changed by commit `6367dd0b` are restored byte-for-byte to their pre-feature contents from `41875379^`:

- `frontend/src/views/admin/AIRelayView.vue`
- `frontend/src/views/admin/__tests__/AIRelayView.spec.ts`

Final verification must show no diff for those files against `41875379^`. If implementation discovers that any other main-site change is required, work stops for explicit user approval before that change is made.

## Plugin Administration UI

The independent plugin application provides route search, platform filtering, pagination, selection and batch deletion, create/edit, relay URL copying, refresh, loading, empty, error, and unauthorized states. It does not import main-site source files or CSS.

The route create/edit dialog includes a full-width "Path mappings" section below the base URL. It reuses the row interaction from `ai-relay-manager`:

- source and target path inputs separated by an arrow;
- add-row command;
- delete icon for each row;
- existing mappings load into editable rows;
- blank rows are excluded from the payload;
- source and target values are normalized before submission.

Placeholders use concrete examples such as `responses/compact` and `api/paas/v4/chat/completions`. The table remains compact and does not add a mapping column; mappings are managed in the editor. Icon-only controls have accessible names, dialogs restore focus, and mapping rows stack on narrow viewports.

## Errors And Security

Configuration validation rejects targets that are absolute URLs, protocol-relative URLs, contain query strings or fragments, or normalize to an empty path. This prevents a route mapping from changing the configured upstream host.

Transparent proxying must not forward hop-by-hop headers such as `Connection`, `Proxy-Connection`, `Keep-Alive`, `Transfer-Encoding`, `TE`, `Trailer`, or `Upgrade`. The plugin continues to use the configured HTTP client and account proxy rather than constructing a separate transport.

No request or response protocol translation is attempted. If an operator maps `responses/compact` to a Chat Completions endpoint, compatibility of the original request body with that endpoint is the upstream provider's responsibility.

## Testing

Backend tests cover:

- normalization and validation of mappings;
- defensive copying in memory and SQL repositories;
- SQL schema, scans, and upserts for `JSONB` mappings;
- exact source matching across optional `/v1` forms;
- mapped URL path replacement and unmapped base URL appending;
- all existing canonical endpoint paths using the shared resolver;
- transparent Responses and Responses Compact proxying;
- request query, headers, body, upstream status, response headers, and streaming body preservation;
- account proxy selection on new endpoints;
- rejection of absolute URL, query, fragment, empty, and duplicate-equivalent targets.

Plugin frontend unit tests cover API error handling, route loading and filtering, loading existing mappings, adding and deleting rows, normalized create/update payloads, empty mapping payloads, batch deletion, and unauthorized states. Plugin host tests verify that `/plugins/ai-relay` serves generated application assets rather than redirecting to `/admin/ai-relay`.

The plugin frontend build, typecheck, and unit tests run from `plugin-service/plugins/ai-relay/frontend`. A final Git comparison verifies that the main-site frontend files match `41875379^` exactly.

## Out Of Scope

- Responses-to-Chat-Completions protocol conversion.
- SSE event conversion or buffering.
- Model mapping or request-body rewriting.
- Cross-host or absolute-URL mapping targets.
- Wildcard or prefix path matching.
- Per-method mappings.
- Main-site AI Relay page, router, menu, component, or style changes.
