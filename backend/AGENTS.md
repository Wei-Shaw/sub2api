# BACKEND KNOWLEDGE BASE

## OVERVIEW
`backend/` is the Go application: server bootstrap, config loading, DI, handlers, services, repositories, shared protocol/client helpers, generated Ent ORM, and SQL migrations.
This is the authoritative layer for runtime rules, persistence architecture, and codegen boundaries.

## STRUCTURE
```text
backend/
├── cmd/jwtgen/         # JWT secret helper binary
├── cmd/server/        # Main binary, setup mode, Wire bootstrap
├── internal/
│   ├── config/        # Config loading and validation
│   ├── domain/        # Shared domain constants and models
│   ├── handler/       # HTTP handlers
│   ├── integration/   # End-to-end backend test flows
│   ├── middleware/    # Shared middleware such as rate limiting
│   ├── model/         # Shared model structs
│   ├── pkg/           # Shared protocol/client/helper packages
│   ├── repository/    # Ent/raw SQL/Redis persistence
│   ├── service/       # Account, auth, gateway, billing, ops, subscriptions
│   ├── server/        # Router and HTTP server wiring
│   ├── setup/         # Setup wizard and auto-setup
│   ├── testutil/      # Backend test fixtures and helpers
│   ├── util/          # Focused utility packages
│   └── web/           # Embedded frontend serving
├── ent/               # Mostly generated code; handwritten work lives in schema/
└── migrations/        # SQL migrations, checksum-protected
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Bootstrap / flags | `cmd/server/main.go` | `--setup`, `--version`, auto-setup, shutdown |
| DI graph | `cmd/server/wire.go`, `internal/*/wire.go` | Wire provider sets |
| Routing / middleware | `internal/server/`, `internal/server/routes/`, `internal/middleware/` | Router setup, route registration, shared middleware |
| Service layer | `internal/service/` | Flat package spanning auth, account, admin, gateway, billing, ops, subscriptions, Sora |
| Persistence | `internal/repository/` | Ent client, Redis caches, raw SQL helpers |
| Config | `internal/config/config.go` | Large config surface; exact behavior lives here |
| Shared helpers / clients | `internal/pkg/`, `internal/util/`, `internal/domain/` | Protocol adapters, logging, validation, shared constants |
| Backend e2e tests | `internal/integration/`, `internal/testutil/` | Integration harnesses, fixtures, HTTP helpers |
| Generated ORM | `ent/` | Do not edit generated output |
| Schema work | `ent/schema/` | See `ent/schema/AGENTS.md` |
| Migration work | `migrations/` | See `migrations/AGENTS.md` |

## CONVENTIONS
- Go version is pinned to `1.26.1`; CI verifies it exactly.
- Keep layer boundaries intact: `handler` and `service` must not import `repository` directly.
- Ent is the ORM, Wire is the DI tool; schema/codegen changes are normal here.
- Frontend embedding is part of backend runtime: frontend build artifacts land in `internal/web/dist`.
- Test scopes use build tags such as `unit`, `integration`, and `e2e`.

## ANTI-PATTERNS
- Editing generated Ent files under `ent/` instead of editing `ent/schema/`.
- Bypassing backend layering by importing persistence directly into `service` or `handler`.
- Changing Ent schema without regenerating code.
- Changing migration history in place.
- Treating setup/auto-setup as optional plumbing; startup behavior depends on it.

## COMMANDS
```bash
cd backend && make build
cd backend && make generate
cd backend && make test
cd backend && make test-unit
cd backend && make test-integration
cd backend && go run ./cmd/server
```

## NOTES
- `internal/service/wire.go` is a high-signal navigation file even when you are not editing DI; it reveals the major backend subsystems.
- `internal/service/` is intentionally broad and flat; use filename prefixes plus `wire.go` to navigate instead of expecting child docs under that subtree.
- `internal/repository/migrations_runner.go` is the source of truth for how SQL files execute at runtime.
- Parent-level backend guidance should cover most subtrees; only migrations and handwritten Ent schema have separate child docs because their local failure modes are special.
- If a change touches auth, gateway routing, or admin behavior, expect to cross `handler`, `service`, and `repository` boundaries even if only one package is edited.
