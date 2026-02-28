# Repository Guidelines

## Project Structure & Module Organization
- `backend/`: Go service. `cmd/server` is the entrypoint, `internal/` contains handlers/services/repositories/server wiring, `ent/` holds Ent schemas and generated ORM code, `migrations/` stores DB migrations, and `internal/web/dist/` is the embedded frontend build output.
- `frontend/`: Vue 3 + TypeScript app. Main folders are `src/api`, `src/components`, `src/views`, `src/stores`, `src/composables`, `src/utils`, and test files in `src/**/__tests__`.
- `deploy/`: Docker and deployment assets (`docker-compose*.yml`, `.env.example`, `config.example.yaml`).
- `openspec/`: Spec-driven change docs (`changes/<id>/{proposal,design,tasksREDACTED.md`).
- `tools/`: Utility scripts (security/perf checks).

## Build, Test, and Development Commands
```bash
make build                           # Build backend + frontend
make test                            # Backend tests + frontend lint/typecheck
cd backend && make build             # Build backend binary
cd backend && make test-unit         # Go unit tests
cd backend && make test-integration  # Go integration tests
cd backend && make test              # go test ./... + golangci-lint
cd frontend && pnpm install --frozen-lockfile
cd frontend && pnpm dev              # Vite dev server
cd frontend && pnpm build            # Type-check + production build
cd frontend && pnpm test:run         # Vitest run
cd frontend && pnpm test:coverage    # Vitest + coverage report
python3 tools/secret_scan.py         # Secret scan
```

## Coding Style & Naming Conventions
- Go: format with `gofmt`; lint with `golangci-lint` (`backend/.golangci.yml`).
- Respect layering: `internal/service` and `internal/handler` must not import `internal/repository`, `gorm`, or `redis` directly (enforced by depguard).
- Frontend: Vue SFC + TypeScript, 2-space indentation, ESLint rules from `frontend/.eslintrc.cjs`.
- Naming: components use `PascalCase.vue`, composables use `useXxx.ts`, Go tests use `*_test.go`, frontend tests use `*.spec.ts`.

## Go & Frontend Development Standards
- Control branch complexity: `if` nesting must not exceed 3 levels. Refactor with guard clauses, early returns, helper functions, or strategy maps when deeper logic appears.
- JSON hot-path rule: for read-only/partial-field extraction, prefer `gjson` over full `encoding/json` struct unmarshal to reduce allocations and improve latency.
- Exception rule: if full schema validation or typed writes are required, `encoding/json` is allowed, but PR must explain why `gjson` is not suitable.

### Go Performance Rules
- Optimization workflow rule: benchmark/profile first, then optimize. Use `go test -bench`, `go tool pprof`, and runtime diagnostics before changing hot-path code.
- For hot functions, run escape analysis (`go build -gcflags=all='-m -m'`) and prioritize stack allocation where reasonable.
- Every external I/O path must use `context.Context` with explicit timeout/cancel.
- When creating derived contexts (`WithTimeout` / `WithDeadline`), always `defer cancel()` to release resources.
- Preallocate slices/maps when size can be estimated (`make([]T, 0, n)`, `make(map[K]V, n)`).
- Avoid unnecessary allocations in loops; reuse buffers and prefer `strings.Builder`/`bytes.Buffer`.
- Prohibit N+1 query patterns; batch DB/Redis operations and verify indexes for new query paths.
- For hot-path changes, include benchmark or latency comparison evidence (e.g., `go test -bench` before/after).
- Keep goroutine growth bounded (worker pool/semaphore), and avoid unbounded fan-out.
- Lock minimization rule: if a lock can be avoided, do not use a lock. Prefer ownership transfer (channel), sharding, immutable snapshots, copy-on-write, or atomic operations to reduce contention.
- When locks are unavoidable, keep critical sections minimal, avoid nested locks, and document why lock-free alternatives are not feasible.
- Follow `sync` guidance: prefer channels for higher-level synchronization; use low-level mutex primitives only where necessary.
- Avoid reflection and `interface{REDACTED`-heavy conversions in hot paths; use typed structs/functions.
- Use `sync.Pool` only when benchmark proves allocation reduction; remove if no measurable gain.
- Avoid repeated `time.Now()`/`fmt.Sprintf` in tight loops; hoist or cache when possible.
- For stable high-traffic binaries, maintain representative `default.pgo` profiles and keep `go build -pgo=auto` enabled.

### Data Access & Cache Rules
- Every new/changed SQL query must be checked with `EXPLAIN` (or `EXPLAIN ANALYZE` in staging) and include index rationale in PR.
- Default to keyset pagination for large tables; avoid deep `OFFSET` scans on hot endpoints.
- Query only required columns; prohibit broad `SELECT *` in latency-sensitive paths.
- Keep transactions short; never perform external RPC/network calls inside DB transactions.
- Connection pool must be explicitly tuned and observed via `DB.Stats` (`SetMaxOpenConns`, `SetMaxIdleConns`, `SetConnMaxIdleTime`, `SetConnMaxLifetime`).
- Avoid overly small `MaxOpenConns` that can turn DB access into lock/semaphore bottlenecks.
- Cache keys must be versioned (e.g., `user_usage:v2:{idREDACTED`) and TTL should include jitter to avoid thundering herd.
- Use request coalescing (`singleflight` or equivalent) for high-concurrency cache miss paths.

### Frontend Performance Rules
- Route-level and heavy-module code splitting is required; lazy-load non-critical views/components.
- API requests must support cancellation and deduplication; use debounce/throttle for search-like inputs.
- Minimize unnecessary reactivity: avoid deep watch chains when computed/cache can solve it.
- Prefer stable props and selective rendering controls (`v-once`, `v-memo`) for expensive subtrees when data is static or keyed.
- Large data rendering must use pagination or virtualization (especially tables/lists >200 rows).
- Move expensive CPU work off the main thread (Web Worker) or chunk tasks to avoid UI blocking.
- Keep bundle growth controlled; avoid adding heavy dependencies without clear ROI and alternatives review.
- Avoid expensive inline computations in templates; move to cached `computed` selectors.
- Keep state normalized; avoid duplicated derived state across multiple stores/components.
- Load charts/editors/export libraries on demand only (`dynamic import`) instead of app-entry import.
- Core Web Vitals targets (p75): `LCP <= 2.5s`, `INP <= 200ms`, `CLS <= 0.1`.
- Main-thread task budget: keep individual tasks below ~50ms; split long tasks and yield between chunks.
- Enforce frontend budgets in CI (Lighthouse CI with `budget.json`) for critical routes.

### Performance Budget & PR Evidence
- Performance budget is mandatory for hot-path PRs: backend p95/p99 latency and CPU/memory must not regress by more than 5% versus baseline.
- Frontend budget: new route-level JS should not increase by more than 30KB gzip without explicit approval.
- For any gateway/protocol hot path, attach a reproducible benchmark command and results (input size, concurrency, before/after table).
- Profiling evidence is required for major optimizations (`pprof`, flamegraph, browser performance trace, or bundle analyzer output).

### Quality Gate
- Any changed code must include new or updated unit tests.
- Coverage must stay above 85% (global frontend threshold and no regressions for touched backend modules).
- If any rule is intentionally violated, document reason, risk, and mitigation in the PR description.

## Testing Guidelines
- Backend suites: `go test -tags=unit ./...`, `go test -tags=integration ./...`, and e2e where relevant.
- Frontend uses Vitest (`jsdom`); keep tests near modules (`__tests__`) or as `*.spec.ts`.
- Enforce unit-test and coverage rules defined in `Quality Gate`.
- Before opening a PR, run `make test` plus targeted tests for touched areas.

## Commit & Pull Request Guidelines
- Follow Conventional Commits: `feat(scope): ...`, `fix(scope): ...`, `chore(scope): ...`, `docs(scope): ...`.
- PRs should include a clear summary, linked issue/spec, commands run for verification, and screenshots/GIFs for UI changes.
- For behavior/API changes, add or update `openspec/changes/...` artifacts.
- If dependencies change, commit `frontend/pnpm-lock.yaml` in the same PR.

## Security & Configuration Tips
- Use `deploy/.env.example` and `deploy/config.example.yaml` as templates; do not commit real credentials.
- Set stable `JWT_SECRET`, `TOTP_ENCRYPTION_KEY`, and strong database passwords outside local dev.
