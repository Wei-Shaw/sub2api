# FRONTEND KNOWLEDGE BASE

## OVERVIEW
`frontend/` is a Vue 3 application built with Vite, Pinia, Vitest, and TailwindCSS, then embedded into the Go backend.
This file owns frontend-wide rules: package management, bootstrap order, router conventions, API client shape, tests, and admin/user/auth UI patterns.

## STRUCTURE
```text
frontend/
├── package.json         # pnpm scripts
├── vite.config.ts       # Build, proxy, manual chunking, embed output path
├── vitest.config.ts     # jsdom tests, coverage thresholds
└── src/
    ├── api/             # Axios client + API modules
    ├── components/      # Shared UI by domain
    ├── composables/     # Reusable view logic
    ├── i18n/            # Translations and i18n bootstrapping
    ├── router/          # Route table, guards, title handling
    ├── stores/          # Pinia stores
    ├── utils/           # Helpers
    └── views/           # Auth, admin, user, setup pages
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| App bootstrap | `src/main.ts`, `src/App.vue` | Theme/init order, injected config, auth side effects |
| Routing / guards | `src/router/index.ts` | Lazy routes, auth/admin/backend-mode/simple-mode rules |
| API client | `src/api/client.ts`, `src/api/index.ts`, `src/api/admin/` | Interceptors, token refresh, admin API surface |
| Shared state | `src/stores/` | Auth, app UI, admin settings, subscriptions, onboarding |
| Shared UI | `src/components/` | Domain buckets such as `common`, `layout`, `account` |
| Reusable logic | `src/composables/` | Forms, OAuth, tables, navigation helpers |
| Auth flow docs | `src/views/auth/README.md` and companions | Existing local docs are strong; don’t duplicate them |

## CONVENTIONS
- Use `pnpm`; CI expects `pnpm install --frozen-lockfile` and the lockfile to stay in sync.
- Build output goes to `../backend/internal/web/dist` because the backend embeds the frontend bundle.
- `src/main.ts` loads injected public settings before mount; respect that startup order when editing bootstrap code.
- Router entries are lazy-loaded and rely on metadata such as `requiresAuth`, `requiresAdmin`, `titleKey`, and `descriptionKey`.
- Tests are colocated in sibling `__tests__/` directories throughout `src/`.
- The admin API layer is centralized under `src/api/admin/index.ts`; prefer extending that pattern instead of ad hoc fetch code.

## ANTI-PATTERNS
- Switching package management to `npm` or changing deps without updating `pnpm-lock.yaml`.
- Bypassing the shared API client/interceptors for normal application requests.
- Breaking the injected-config-before-mount flow in `src/main.ts`.
- Duplicating route-title logic in components instead of using router metadata and title helpers.
- Creating subtree docs that restate frontend-wide rules already covered here or in existing README files.

## COMMANDS
```bash
cd frontend && pnpm install
cd frontend && pnpm run dev
cd frontend && pnpm run build
cd frontend && pnpm run lint:check
cd frontend && pnpm run typecheck
cd frontend && pnpm run test:run
cd frontend && pnpm run test:coverage
```

## NOTES
- Existing local docs are already strong in `src/router/`, `src/stores/`, `src/components/common/`, `src/components/layout/`, and `src/views/auth/`; use them instead of copying them into new AGENTS files.
- Keep admin and admin/ops work under this parent unless truly local rules emerge; current evidence shows shared router/store/API conventions matter more than directory size.
- `views/admin/ops/` is deep, but its rules still depend on shared frontend patterns such as admin routing, centralized admin APIs, and store-driven UI state.
