# Task Plan: Codebase Explanation Document

## Goal
Produce a concise code explanation document for second-stage development, covering frontend/backend structure and the core logic for key management, account pool, and per-session cost calculation.

## Current Phase
Phase 5

## Phases

### Phase 1: Requirements & Discovery
- [x] Capture user requirements
- [x] Identify project structure and documentation conventions
- [x] Document initial findings in findings.md
- **Status:** complete

### Phase 2: Backend Code Analysis
- [x] Map backend modules, routing, services, persistence, and pricing logic
- [x] Trace key management logic
- [x] Trace account pool logic
- [x] Trace per-session cost calculation logic
- **Status:** complete

### Phase 3: Frontend Code Analysis
- [x] Map frontend routes, stores, API client, and relevant management screens
- [x] Connect frontend operations to backend APIs
- **Status:** complete

### Phase 4: Write Documentation
- [x] Create a code explanation document in docs/
- [x] Include code references and development notes
- **Status:** complete

### Phase 5: Verification & Delivery
- [x] Review the document against source code
- [x] Report files changed and any uncertainty
- **Status:** complete

## Key Questions
1. How are API keys created, stored, validated, listed, updated, and revoked?
2. How is the account pool represented, selected, refreshed, disabled, and recovered?
3. How is each session or request cost calculated, persisted, and exposed?
4. Which frontend pages/stores call these backend capabilities?

## Decisions Made
| Decision | Rationale |
|----------|-----------|
| Write a standalone document under docs/ | Existing project already uses docs/ for operational and integration documentation. |

## Errors Encountered
| Error | Attempt | Resolution |
|-------|---------|------------|
| `python` command not found while running session-catchup | 1 | Re-ran the script with `python3`; no catch-up output was produced. |
| `sed backend/internal/service/account_pool_mode.go` file not found | 1 | Located account scheduling logic in `gateway_service.go`, `scheduler_snapshot_service.go`, and `account_repo.go`. |
| `sed backend/internal/service/admin_service_apikey.go` file not found | 1 | Located admin API key handler in `backend/internal/handler/admin/apikey_handler.go`; service methods are in admin service files. |

## Notes
- User wants a code explanation document for second-stage development.
- Core logic that must be covered: key management, account pool, per-session cost calculation.
- Verification confirmed the document includes the requested key sections. `docs/*` is ignored by `.gitignore`, so the new document exists locally but does not appear in normal `git status`.
