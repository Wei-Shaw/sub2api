# Codex catalog freshness

The API-key modal fetches its default catalog from `GET /backend-api/codex/models`.
That route omits `client_version`, so the backend applies the runtime canonical
Codex version maintained by `OpenAICodexVersionSyncService`.  A user can opt
into an explicit version only for client compatibility; that request continues
to use `/v1/models?client_version=<version>`.

Catalogs remain scoped to the requesting API key's group and model allowlist.
The downloaded `codex-models.json` is a local snapshot: a new platform catalog
does not rewrite files already downloaded by a CLI or desktop client.

The version synchronizer checks stable releases every six hours. An administrator
override takes precedence over the synchronized value; a failed sync retains the
last known valid version.

## Delivery checks

Run these checks from `frontend/`:

```sh
pnpm exec vitest run src/api/__tests__/codex.spec.ts src/components/keys/__tests__/UseKeyModal.spec.ts
pnpm run typecheck
pnpm run build
```

Build the application image from the repository root, using a unique image tag
for `CATALOG_IMAGE`:

```sh
docker build -t "$CATALOG_IMAGE" .
```

The supplied Compose files use a fixed image reference; setting `IMAGE` alone
does not change it. For an approved rollout, set `DEPLOY_COMPOSE` to the active
deployment file, `APP_SERVICE` to its application service, and `CATALOG_IMAGE`
to the tested image. Use the same Compose project name as the running deployment
(set `COMPOSE_PROJECT_NAME` if it is not inferred from that file).

```sh
APP_CONTAINER=$(docker compose -f "$DEPLOY_COMPOSE" ps -q "$APP_SERVICE")
ROLLBACK_IMAGE=$(docker inspect --format '{{.Image}}' "$APP_CONTAINER")
CATALOG_OVERRIDE=$(mktemp)
printf 'services:\n  %s:\n    image: %s\n' "$APP_SERVICE" "$CATALOG_IMAGE" > "$CATALOG_OVERRIDE"
docker compose -f "$DEPLOY_COMPOSE" -f "$CATALOG_OVERRIDE" config --images
docker compose -f "$DEPLOY_COMPOSE" -f "$CATALOG_OVERRIDE" up -d --no-deps --pull never "$APP_SERVICE"
```

Retain the old local image and the override path until acceptance. Roll back
only the application service if acceptance fails:

```sh
printf 'services:\n  %s:\n    image: %s\n' "$APP_SERVICE" "$ROLLBACK_IMAGE" > "$CATALOG_OVERRIDE"
docker compose -f "$DEPLOY_COMPOSE" -f "$CATALOG_OVERRIDE" up -d --no-deps --pull never "$APP_SERVICE"
```

After rollout, verify that a fresh Codex modal request
uses `/backend-api/codex/models`, that an allowed group receives its complete
catalog, and that a second group remains restricted to its own allowlist.
