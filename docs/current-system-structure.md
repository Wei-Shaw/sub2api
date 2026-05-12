# Current System Structure And Handoff

Last updated: 2026-05-04, Asia/Shanghai.

This document is a handoff note for continuing work in a new session. It summarizes the current architecture, the managed-user internal Key work, and the local environment state.

## Repository Layout

- `backend/`: Go service, Gin HTTP server, Ent ORM, PostgreSQL migrations, Redis-backed caches and runtime locks.
- `frontend/`: Vue 3 admin/user UI, Vite dev server, admin pages and API clients.
- `deploy/`: Docker Compose examples and production deployment templates.
- `docs/`: project documentation and this handoff document.

## Runtime Architecture

The service is a single Go backend plus a Vue frontend.

- Frontend dev server runs on `http://127.0.0.1:3000`.
- Backend API runs on `http://127.0.0.1:8080`.
- PostgreSQL stores users, API keys, balances, groups, channels, logs, settings, and migrations.
- Redis stores short-lived auth/cache state, billing cache, IP locks, concurrency and rate-limit coordination.
- Gateway requests authenticate through API Key middleware, then route into provider-specific gateway handlers.
- Admin APIs use JWT admin auth under `/api/v1/admin/*`.

## Managed User Internal Key Model

The internal customer model now uses "managed users":

- `users.customer_type` distinguishes normal users from managed internal customers.
- `retail` means ordinary self-service users.
- `managed` means an internal customer created and operated by an admin.
- A managed customer does not receive login credentials.
- Admin creates the managed user with random email/password, stores the customer name in username, and stores contact/notes as user profile fields where supported.
- API keys remain the operational unit for delivery, quotas, groups, concurrency, RPM, expiry, IP lock, and soft throttling.

Key data reuse:

- `users.balance` is the managed customer's internal balance pool.
- `api_keys.quota` is the single-key quota.
- `usage_logs.user_id`, `usage_logs.api_key_id`, and `usage_logs.group_id` continue tracking usage.
- Existing groups, group rates, channel pricing, and billing caches continue to apply.

## Database Changes

Migration:

- `backend/migrations/135_managed_users_api_key_policies.sql`

Important columns added:

- `users.customer_type`
- `api_keys.ip_lock_mode`
- `api_keys.limit_action`
- `api_keys.rate_limit_1mo`
- `api_keys.usage_1mo`
- `api_keys.window_1mo_start`

Generated Ent code was regenerated after schema edits.

Current local restored database status:

```text
users=14
api_keys=18
schema_135=1
managed_cols=6
```

## Admin Managed Key APIs

Implemented endpoints:

- `GET /api/v1/admin/managed-keys`
- `POST /api/v1/admin/managed-keys`
- `GET /api/v1/admin/managed-keys/:id/delivery`
- `POST /api/v1/admin/managed-keys/:id/reset-ip-lock`

Core backend files:

- `backend/internal/handler/admin/apikey_handler.go`
- `backend/internal/server/routes/admin.go`
- `backend/internal/service/api_key_service.go`
- `backend/internal/repository/api_key_repo.go`
- `backend/internal/server/middleware/api_key_auth.go`
- `backend/internal/server/middleware/api_key_auth_google.go`

## Access Control

Managed keys default to:

- `ip_lock_mode=auto_single_ip`
- `limit_action=soft_throttle`

IP lock behavior:

- First trusted client IP binds the key.
- Same IP refreshes the Redis lock TTL.
- Different IP during TTL receives ambiguous `403 ACCESS_DENIED`.
- Admin reset deletes the Redis IP lock.

Redis IP lock key prefix:

```text
apikey:iplock:
```

Soft throttle behavior:

- Existing 5-hour and daily limits are preserved.
- Monthly limit is added through `rate_limit_1mo`.
- If `limit_action=soft_throttle`, over-limit requests are delayed before gateway dispatch instead of immediately rejected.
- Delay currently starts at about 2 seconds and is capped at about 60 seconds.

## Frontend

Admin page:

- `frontend/src/views/admin/ManagedKeysView.vue`

Frontend API client:

- `frontend/src/api/admin/apiKeys.ts`

Route:

- `/admin/managed-keys`

Related UI/navigation files:

- `frontend/src/router/index.ts`
- `frontend/src/components/layout/AppSidebar.vue`
- `frontend/src/types/index.ts`
- `frontend/src/i18n/locales/zh.ts`
- `frontend/src/i18n/locales/en.ts`

Homepage split:

- Public users use the static `/home` page implemented in frontend code.
- Internal customers use the static `/internal-home` page implemented in frontend code.
- API onboarding uses the static `/docs` page implemented in frontend code.
- Agent prompt browsing uses the static `/agents` Agents Hub page implemented in frontend code.
- Different domains can be mapped to the internal customer homepage through `internal_home_domains`; matching domains redirect `/` to `/internal-home`, while all other domains redirect `/` to `/home`.
- Homepage content is no longer configured through `home_content` or `internal_home_content`; those pages are treated as code-owned static sites.

The page supports:

- Managed key list.
- Customer name/contact/notes.
- Group, concurrency, RPM, quota, expiry.
- 5-hour/day/month limits.
- IP whitelist/auto lock policy fields.
- Soft throttle policy.
- Delivery dialog with API key, authorization header, OpenAI/Claude base URL, and Gemini base URL.
- IP lock reset.

## Local Environment State

The local database was replaced with a dump from `la-vps`.

Remote source:

- SSH host: `la-vps`
- Remote hostname observed: `racknerd-e11fe38`
- Remote containers: `sub2api`, `sub2api-postgres`, `sub2api-redis`
- Remote PostgreSQL version: `18.3`

Local runtime:

- PostgreSQL 18 installed through Homebrew.
- Redis installed through Homebrew.
- Local PostgreSQL data dir: `/tmp/sub2api-runtime/postgres18`
- Local Redis data dir: `/tmp/sub2api-runtime/redis`
- App config/data dir: `/tmp/sub2api-runtime/app`
- Local database name/user/password: `sub2api` / `sub2api` / `sub2api`

Backups:

- VPS dump: `/tmp/sub2api-runtime/backups/la-vps-sub2api-20260504-095107.dump`
- Local pre-restore backup: `/tmp/sub2api-runtime/backups/local-before-la-vps-20260504-095029.dump`

Important: the old temporary local admin password was overwritten by the VPS data. Use the VPS admin credentials when logging into the local UI.

## Start Commands

Start PostgreSQL 18:

```bash
/opt/homebrew/opt/postgresql@18/bin/postgres -D /tmp/sub2api-runtime/postgres18 -h 127.0.0.1 -p 5432
```

Start Redis:

```bash
/opt/homebrew/opt/redis/bin/redis-server --bind 127.0.0.1 --port 6379 --dir /tmp/sub2api-runtime/redis --save "" --appendonly no
```

Start backend:

```bash
cd /Users/wangzihao/Projects/sub2api/backend
DATA_DIR=/tmp/sub2api-runtime/app TZ=Asia/Shanghai go run ./cmd/server
```

Start frontend:

```bash
cd /Users/wangzihao/Projects/sub2api/frontend
VITE_DEV_PROXY_TARGET=http://127.0.0.1:8080 VITE_DEV_PORT=3000 npm run dev -- --host 127.0.0.1 --port 3000
```

Health check:

```bash
curl -sS http://127.0.0.1:8080/health
```

Expected:

```json
{"status":"ok"}
```

## Verification Already Run

Backend:

```bash
cd backend && go test ./...
```

Frontend:

```bash
cd frontend && npm run typecheck
cd frontend && npm run build
```

After restoring the VPS database:

```bash
curl -sS http://127.0.0.1:8080/health
/opt/homebrew/opt/postgresql@18/bin/psql -h 127.0.0.1 -p 5432 -U sub2api -d sub2api -tAc "select 'users=' || count(*) from users; select 'api_keys=' || count(*) from api_keys; select 'schema_135=' || count(*) from schema_migrations where filename='135_managed_users_api_key_policies.sql';"
```

## Current Git Worktree Notes

There are many modified files from the managed-user implementation, including generated Ent files. Do not assume unrelated changes are disposable.

New files:

- `backend/migrations/135_managed_users_api_key_policies.sql`
- `backend/internal/service/api_key_service_ip_lock_test.go`
- `frontend/src/views/admin/ManagedKeysView.vue`
- `docs/current-system-structure.md`

Broad modified areas:

- Ent schemas and generated files for `User` and `APIKey`.
- Admin API key handler and admin routes.
- API key service, repository, auth cache, and auth middleware.
- Billing cache and usage billing repository for monthly window support.
- Frontend managed key API client, route, sidebar, i18n, and shared types.

## Suggested Next Work

- Verify managed-key creation end to end in the browser using the restored VPS data.
- Decide whether the internal platform should be a separate deployment mode or a UI feature flag layered on current admin settings.
- Add focused integration tests for managed-key creation and monthly soft throttle behavior if more confidence is needed.
- Review security wording and delivery copy before sending internal customer documentation.
- Consider moving the runtime handoff commands into a script if local testing will be repeated often.
