# Findings & Decisions

## Requirements
-梳理当前项目的前端和后端代码。
-整理一个代码说明文档，面向二次开发。
-重点解释核心逻辑：秘钥管理、账号池、每次会话费用计算。

## Research Findings
- Project root contains `backend/`, `frontend/`, `deploy/`, and `docs/`.
- Backend appears to be Go-based (`backend/go.mod`).
- Frontend appears to be Vue/Vite/Tailwind-based (`frontend/package.json`, `frontend/vite.config.ts`, `frontend/tailwind.config.js`).
- README_CN states the product is an AI API gateway for distributing and managing AI subscription quotas. Users call upstream AI services via platform-generated API keys; the platform handles auth, billing, load balancing, and proxying.
- README_CN lists key features directly relevant to this document: multi-account management, API key distribution, token-level usage tracking/cost calculation, intelligent account selection with sticky sessions, user/account concurrency limits, and rate limits.
- Backend dependencies confirm Gin (`github.com/gin-gonic/gin`), Ent (`entgo.io/ent`), PostgreSQL driver (`github.com/lib/pq`), Redis (`github.com/redis/go-redis/v9`), decimal arithmetic (`github.com/shopspring/decimal`), and go-cache/Ristretto caching.
- `backend/go.mod` currently declares `go 1.26.2`, while README badges/tech stack still mention Go 1.25.7.
- Ent schemas include `api_key`, `account`, `account_group`, `usage_log`, `user`, `user_subscription`, `subscription_plan`, and payment-related entities, so the core data model is Ent-first.
- Runtime assembly uses Wire provider sets: config, repository, service, payment, middleware, handler, and server are composed from `backend/cmd/server/wire.go`.
- Gin route registration starts in `backend/internal/server/router.go`: `/api/v1/*` hosts auth/user/admin/payment APIs, while gateway-compatible paths are registered directly on `/v1`, `/v1beta`, `/responses`, `/chat/completions`, `/images/*`, and `/backend-api/codex`.
- API key auth accepts `Authorization: Bearer`, `x-api-key`, and Gemini-compatible `x-goog-api-key`; query-string keys are rejected.
- API key auth always checks key existence/status, IP restrictions, user existence/status, and group context. Billing enforcement then checks key expiry/quota, subscription limits, or user balance depending on group mode.
- API key auth has L1 Ristretto + L2 Redis cache keyed by SHA-256 of the raw key, with negative caching, singleflight, jittered TTL, and Redis pub/sub invalidation.
- User API key CRUD is exposed by `frontend/src/api/keys.ts` and `/api/v1/keys`; the user UI is `frontend/src/views/user/KeysView.vue`.
- Account pool membership is the many-to-many `accounts` ↔ `groups` relationship through `account_groups`; accounts carry platform/type/credentials/proxy/concurrency/priority/status/schedulable/limit-window fields.
- Scheduler snapshots are maintained by `SchedulerSnapshotService`, backed by `SchedulerCache` and outbox events. Gateway selection prefers snapshot reads and falls back to DB with throttling.
- Gateway account selection considers group/platform, model support, model routing, sticky session, excluded failed accounts, account schedulability, quota/window/RPM checks, concurrency load, priority, and LRU.
- Sticky session bindings are stored in Redis as `sticky_session:{groupID}:{sessionHash}` with 1-hour TTL and group isolation.
- Usage cost calculation is centralized in `BillingService` and `GatewayService.recordUsageCore`.
- Cost formula for token mode is: raw component costs from tokens and model prices, optional service-tier/long-context/cache-breakdown adjustments, then `actual_cost = total_cost * rate_multiplier`.
- Rate multiplier precedence in gateway usage recording is system default, then group default, then user-specific group multiplier when configured.
- Production billing uses `UsageBillingRepository.Apply`, which deduplicates by `(request_id, api_key_id)` plus a request fingerprint before applying subscription usage, balance deduction, API key quota/rate-limit usage, and account quota usage in one DB transaction.
- Frontend core routes: `/keys`, `/usage`, `/admin/accounts`, `/admin/groups`, `/admin/usage`, `/admin/channels/pricing`.

## Technical Decisions
| Decision | Rationale |
|----------|-----------|
| Use source-code tracing rather than README-only summary | The requested output is for second-stage development and must explain implementation logic. |

## Issues Encountered
| Issue | Resolution |
|-------|------------|
| `python` binary unavailable | Used `python3` for the planning skill catch-up script. |

## Resources
- `README.md`
- `README_CN.md`
- `backend/go.mod`
- `frontend/package.json`
- `backend/ent/schema/api_key.go`
- `backend/ent/schema/account.go`
- `backend/ent/schema/usage_log.go`
- `backend/internal/server/middleware/api_key_auth.go`
- `backend/internal/service/api_key_service.go`
- `backend/internal/service/gateway_service.go`
- `backend/internal/service/billing_service.go`
- `backend/internal/repository/usage_billing_repo.go`
- `frontend/src/api/keys.ts`
- `frontend/src/api/admin/accounts.ts`
- `frontend/src/api/admin/groups.ts`
- `frontend/src/api/usage.ts`
- `frontend/src/api/admin/usage.ts`

## Visual/Browser Findings
- No browser or visual inspection used so far.
