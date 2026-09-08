# Sub2API Plugin Service

This service is now a unified, independently deployable plugin host. It keeps shared auth/session handling in one place and mounts each plugin through a standard structure:

- `plugins/<plugin-key>/manifest`
- `plugins/<plugin-key>/backend`
- `plugins/<plugin-key>/web`

The first hosted plugin is `image-generation`.

## Runtime

```bash
cd plugin-service
go test ./...
go run ./cmd/server
```

Default local listener: `http://localhost:8091`.

Configure the plugin entry base URL in the main Sub2API custom menu, not in the
plugin service. Keep the original absolute URL shape, using `plugin-server` as
the fixed placeholder host:

- `http://plugin-server/plugins/image-generation`

The main frontend turns this entry into the same-origin plugin URL:

- `/plugins/image-generation`

For deployment, keep the browser-facing entry on the main site domain. Configure
the plugin service port in the shared deployment env, for example:

```env
PLUGIN_SERVER_PORT=8091
```

Then reverse-proxy this path prefix from the main site domain to the fixed
internal plugin service host in the same Docker network or environment:

- `/plugins/*`

The plugin service reuses the same PostgreSQL configuration as the main site.
If `DATABASE_URL` or the shared `DATABASE_*` environment variables are present,
image generation history is persisted into `plugin_generation_history`;
otherwise it falls back to in-memory history for local-only runs.

Image media can be persisted in a private MinIO bucket. Configure
`MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `MINIO_BUCKET`, and
optionally `MINIO_USE_SSL`. When all required values are present, uploaded
reference images and generated results are stored as MinIO objects while
history keeps stable authenticated plugin URLs. Partial MinIO configuration is
rejected at startup. The maintained Docker Compose files include MinIO and a
persistent data volume; replace the example root credentials before production
deployment and include the MinIO volume in backups.

For a plugin service started directly on the Docker host, set
`MINIO_ENDPOINT=127.0.0.1:9000`. Compose publishes the MinIO API on
`${MINIO_API_PORT:-9000}` and its console on `${MINIO_CONSOLE_PORT:-9001}`,
both bound to loopback by default. A plugin service running in Compose must
continue to use the internal endpoint `minio:9000`.

## Auth Model

The plugin host fully reuses the main Sub2API login state. It does not create
its own session cookie, does not persist plugin login state, and does not
provide a standalone development login entry.

Embed flow:

1. Main custom menu stores `http://plugin-server/plugins/image-generation` as the plugin base URL.
2. Main frontend opens `/plugins/image-generation` on the same domain.
3. Browser carries the current Sub2API token query parameter or same-site cookie.
4. Every protected plugin API request forwards those credentials to the main site `/api/v1/auth/me`.

The main site base URL for auth verification is discovered automatically inside
the deployment environment. The plugin host tries these candidates in order:

- `http://sub2api:8080`
- `http://localhost:8080`
- `http://127.0.0.1:8080`

The standalone host enforces role-aware history access:

- `admin` can list and read all plugin history records.
- `user` can list and read only records matching their `user_id`.

## Plugin Host Layout

- Host routes:
  - `GET /healthz`
- Hosted image-generation page:
  - `GET /plugins/image-generation`
  - `GET /plugins/image-generation/assets/app.css`
  - `GET /plugins/image-generation/assets/app.js`
- Namespaced image-generation APIs:
  - `GET /plugins/image-generation/api/me`
  - `GET /plugins/image-generation/api/config`
  - `POST /plugins/image-generation/api/generate`
  - `GET /plugins/image-generation/api/creations`
  - `GET /plugins/image-generation/api/history`
  - `GET /plugins/image-generation/api/history/{id}`
  - `POST /plugins/image-generation/api/history/{id}/retry`
  - `POST /plugins/image-generation/api/history/{id}/cancel`

## Image Generation Frontend Development

The image-generation UI is maintained as an independent Vue and TypeScript project:

```text
plugins/image-generation/frontend/
```

Edit application behavior only under `frontend/src`. The files in `web/assets` are committed build outputs used by the Go host and must not be edited directly.

From `plugin-service/plugins/image-generation/frontend`, run:

```bash
npm install
npm test
npm run typecheck
npm run build
npm run verify:generated
```

The production build writes deterministic `app.js` and `app.css` files to `../web/assets` while preserving the hosted URLs under `/plugins/image-generation/assets/`.

`POST /plugins/image-generation/api/generate` proxies image generation requests to the main Sub2API gateway resolved from the same-origin request headers. The request must include the authenticated user's numeric `api_key_id`. The plugin service resolves the key secret only while calling the gateway, persists the ID with structured history, and returns compact job status responses until the final result is ready.

## Adding a New Plugin

1. Copy `plugins/image-generation/` as the starting template.
2. Update the manifest metadata and entry path.
3. Implement plugin backend handlers and register their routes.
4. Add hosted frontend assets under `web/`.
5. Register the plugin from `plugins.RegisterAll`; the host router discovers routes through the registry.

## Agnes Image Relay

The `ai-relay` plugin exposes an internal OpenAI-compatible relay for Agnes.
Configure routes as an administrator from the main-site AI Relay page:

- `/admin/ai-relay`

The route configuration stores only the platform, name, slug, and target base
URL. It does not store an API key. In main-site account management, use:

- `http://plugin-server:8091/plugins/ai-relay/agnes/<slug>`

The main site forwards that account's Bearer key to the plugin, and the plugin
forwards it only to the configured Agnes endpoint. The relay supports image
generation, image editing, model listing, and chat completions through their
OpenAI-compatible `/v1` paths.

When the account has a primary proxy, the main backend sends only its numeric
proxy ID over the internal plugin request. The plugin resolves credentials from
the shared PostgreSQL `proxies` table and applies the proxy only between the
plugin and Agnes. The main-site-to-plugin hop remains direct. Missing, disabled,
expired, unsupported, or unreachable configured proxies return an upstream
error and never fall back to a direct Agnes connection.

Keep port `8091` internal and configure the plugin service with the same
PostgreSQL settings as the main service. Proxy usernames and passwords are not
sent to browsers or included in main-to-plugin headers.
