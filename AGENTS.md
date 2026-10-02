# PROJECT KNOWLEDGE BASE

**Generated:** 2026-03-29 21:13:36 CST
**Commit:** 941a44da
**Branch:** feat/agents-md

## OVERVIEW
Sub2API is an AI API gateway platform: Go backend for routing, billing, auth, and persistence; Vue frontend for admin/user UI; Docker/systemd deploy assets for self-hosting.
Core stack: Go 1.26.1 + Gin + Ent + Wire + PostgreSQL/Redis, plus Vue 3 + Vite + Pinia + Vitest + TailwindCSS.

## STRUCTURE
```text
curious-cactus/
├── backend/      # Go runtime, persistence, generated Ent, SQL migrations
├── frontend/     # Vue app; build output embeds into backend/internal/web/dist
├── deploy/       # Docker/systemd install assets and config templates
├── .github/      # CI, security scan, release workflow
├── docs/         # Product/developer docs
└── tools/        # Repo utilities such as secret/audit helpers
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Server bootstrap | `backend/cmd/server/main.go` | Setup mode, auto-setup, graceful shutdown |
| HTTP routing | `backend/internal/server/router.go` | Registers auth, user, admin, gateway routes |
| Business logic | `backend/internal/service/` | Covered by `backend/AGENTS.md` |
| Persistence | `backend/internal/repository/` | Ent + raw SQL + Redis caches |
| Schema edits | `backend/ent/schema/` | Covered by `backend/ent/schema/AGENTS.md` |
| Migration edits | `backend/migrations/` | Covered by `backend/migrations/AGENTS.md` |
| Frontend bootstrap | `frontend/src/main.ts` | Injected config before mount; router/i18n setup |
| Frontend routing | `frontend/src/router/index.ts` | Lazy routes, auth/admin/backend-mode guards |
| Admin API/UI | `frontend/src/api/admin/`, `frontend/src/views/admin/` | Covered by `frontend/AGENTS.md` |
| Deployment / self-hosting | `deploy/` | See `deploy/AGENTS.md` for compose variants, install scripts, and socket/path gotchas |
| Repo tooling | `tools/` | Small helper scripts such as audit/exception checks |
| Product/API docs | `docs/` | Start here for integration-facing docs before editing code |

## CODE MAP
| Symbol | Type | Location | Role |
|--------|------|----------|------|
| `main` | function | `backend/cmd/server/main.go` | Entry point for setup mode and normal server mode |
| `runMainServer` | function | `backend/cmd/server/main.go` | Loads config, initializes app, starts server |
| `SetupRouter` | function | `backend/internal/server/router.go` | Applies middleware and mounts route groups |
| `ProviderSet` | variable | `backend/internal/service/wire.go` | Service-layer dependency graph root |
| `bootstrap` | function | `frontend/src/main.ts` | Frontend startup sequence |
| `router` | constant | `frontend/src/router/index.ts` | Route table + navigation guards + prefetch |

## CONVENTIONS
- Use `pnpm`, not `npm`; CI installs frontend deps with `pnpm install --frozen-lockfile`.
- Frontend build output goes to `backend/internal/web/dist`; the backend can embed and serve it.
- Backend architecture is layered: `handler` and `service` must not import `repository` directly; lint enforces this.
- Ent output is generated; hand-written ORM work belongs in `backend/ent/schema/` and related mixins.
- SQL migrations auto-run on startup and are checksum-validated for immutability.
- Go version is pinned in CI and local tooling to `1.26.1`.

## ANTI-PATTERNS (THIS PROJECT)
- Editing generated Ent files under `backend/ent/`.
- Modifying an already-applied migration instead of creating a new migration.
- Switching frontend package management to `npm` or changing `package.json` without updating `pnpm-lock.yaml`.
- Using API keys in query parameters; use the `Authorization` header.
- Relying on Sora-related features in production.

## UNIQUE STYLES
- Frontend tests live in sibling `__tests__/` directories; backend tests are colocated as `*_test.go` with build tags such as `unit` and `integration`.
- Route metadata uses `titleKey`/`descriptionKey` for translated titles and page descriptions.
- Some frontend folders already carry rich README-style docs (`views/auth`, `router`, `stores`, `components/common`, `components/layout`); do not duplicate them blindly.
- Docker deployment favors `docker-compose.local.yml` because data stays in local directories and can be migrated by archiving the deploy folder.

## COMMANDS
```bash
make build
make test

cd backend && make generate
cd backend && make test-unit
cd backend && make test-integration

cd frontend && pnpm install
cd frontend && pnpm run build
cd frontend && pnpm run lint:check
cd frontend && pnpm run typecheck
cd frontend && pnpm run test:run
```

## NOTES
- First-run setup can happen through the setup wizard or Docker auto-setup; do not assume a ready config exists.
- For self-hosted stability, keep `JWT_SECRET` and `TOTP_ENCRYPTION_KEY` fixed instead of letting them change every restart.
- When fronting Sub2API with Nginx for Codex CLI traffic, enable `underscores_in_headers on;`.
- Child instruction files exist at `backend/`, `backend/migrations/`, `backend/ent/schema/`, `frontend/`, and `deploy/`; prefer the nearest one.
