# OP Backoffice Design Proposal

## Context

Sub2API already has a broad admin console for configuration and management:

- Users, API keys, groups, subscriptions, platform quotas, and user attributes
- Upstream accounts, proxies, channels, channel monitors, scheduled tests, and TLS profiles
- Usage dashboards, request drilldown, ops errors, alert rules, alert events, and system logs
- Payment orders, plans, payment providers, redeem codes, promo codes, announcements, affiliates, and risk control
- Backup, data management, system updates, and compliance acknowledgement

The OP backoffice should not duplicate those CRUD pages. It should sit above the existing admin modules as an operations workflow layer for customer support, business operations, and on-call handling.

## Goals

- Give operators one place to answer: what needs attention now?
- Turn scattered admin actions into guided workflows for user support, account-pool health, payment follow-up, and incident triage.
- Reuse existing `/admin/*` capabilities where possible.
- Keep risky system configuration in the existing admin pages unless an OP workflow needs a narrow action.
- Prepare for role-based access control and audit logging before exposing sensitive actions to non-super-admin operators.

## Non-Goals

- Replacing the current admin console.
- Rebuilding existing user, account, payment, usage, and ops pages as separate CRUD screens.
- Introducing a second frontend application.
- Adding broad automation that changes balances, account scheduling, or payment state without operator review in the initial phase.

## Recommended Product Shape

Add an OP workspace inside the existing Vue admin app, for example under `/admin/op` or `/admin/workbench`.

The workspace should be task-oriented:

- Workbench overview
- User 360
- Account pool health
- Exception queue
- Payment and revenue operations
- Campaign operations

The existing admin pages remain the source of truth for deep configuration. OP pages link into them for advanced editing.

## MVP Scope

### 1. Workbench Overview

Provide a compact daily operations page that aggregates:

- Today's revenue, paid orders, failed orders, and pending manual follow-up
- Today's requests, tokens, actual cost, account cost, and estimated gross margin
- Active users, new users, high-spend users, and abnormal spenders
- Account availability, errored accounts, temporarily unschedulable accounts, and quota-risk accounts
- Open alert events, unresolved request errors, unresolved upstream errors, and system log ingestion health

Most of this can be composed from existing dashboard, payment, account, and ops APIs. If the frontend would otherwise issue many heavy requests, add a backend snapshot endpoint that calls existing services and returns an OP-specific aggregate DTO.

### 2. Exception Queue

Create a single queue for operator handling:

- Alert events
- Client-visible request errors
- Upstream errors
- Account errors
- Payment exceptions
- Risk-control logs that require review

Each item should show severity, status, source, affected user/account/order, first seen, last seen, count, and available actions.

Initial actions:

- Mark resolved or ignored
- Add operator note
- Open related user, account, request, order, or alert detail
- For account issues: clear error, run test, refresh credentials, clear temporary unschedulable status where already supported
- For user issues: open User 360

### 3. User 360

Provide a support-first user detail surface:

- Profile, status, role, groups, custom attributes, auth identities
- Balance, balance history, subscriptions, platform quotas, and RPM status
- API keys and recent usage
- Recent orders, promo/redeem usage, affiliate relationship
- Recent request errors and risk-control logs
- Safe quick actions: adjust balance with reason, replace group, assign/extend subscription, reset platform quota window, unban user, add internal note

All mutation actions should require reason input and write audit logs once audit storage exists.

### 4. Account Pool Health

Provide an operations view for upstream supply:

- Availability by platform, group, channel, and proxy
- Error accounts, rate-limited accounts, quota-risk accounts, and temporarily unschedulable accounts
- Cost, request volume, success rate, latency, and quota usage by account
- Batch actions that already exist: test, refresh, clear error, refresh tier, reset quota, bulk update selected safe fields

This page should guide on-call operators to restore capacity quickly without exposing unrelated credential or routing configuration by default.

## Later Phases

### Payment and Revenue Operations

- Order follow-up queue for unpaid, failed, cancelled, and provider-mismatched orders
- Manual reconciliation status and operator notes
- Revenue, cost, gross margin, ARPU, first-payment conversion, repurchase rate
- Revenue by plan, group, model, platform, and payment provider

### Campaign Operations

- Campaign wrapper around announcements, promo codes, redeem codes, and affiliate settings
- Targeting by group, activity, spend, registration date, and subscription status
- Conversion tracking: sent, viewed, redeemed, paid, retained

### RBAC

Introduce roles before broad OP adoption:

- Super admin: full existing admin access
- Operator: OP workbench, exception queue, user support actions
- Finance: payment, order, revenue, refund/reconciliation actions
- On-call: account pool health, ops alerts, account recovery actions
- Marketing: campaigns, announcements, promo/redeem codes, affiliate settings

Use deny-by-default permissions for dangerous mutations.

### Audit Logs

Add a durable audit log for sensitive actions:

- Actor ID, role, auth method, IP, user agent
- Action, target type, target ID
- Before/after summary for changed fields
- Reason, request ID, created at

Priority actions to audit:

- Balance changes
- Subscription assignment, extension, revocation, and quota reset
- User status, role, group, RPM, and platform quota changes
- Account status, scheduling, error clearing, credential refresh, and quota reset
- Payment order cancellation, manual reconciliation, and refund markers
- Promo/redeem generation and bulk changes
- Risk-control unban and flagged-hash deletion

## Backend Direction

Prefer adding a thin OP aggregation layer over duplicating business logic:

- `backend/internal/server/routes/admin.go`: add an OP route group if new endpoints are needed.
- `backend/internal/handler`: add handlers that return OP-specific aggregate DTOs.
- `backend/internal/service`: add an OP service that composes existing admin, dashboard, payment, account, ops, usage, affiliate, and risk-control services.
- `backend/internal/repository`: only add repository queries when existing service APIs cannot provide the data efficiently.

Potential endpoint shape:

- `GET /api/v1/admin/op/overview`
- `GET /api/v1/admin/op/exceptions`
- `PUT /api/v1/admin/op/exceptions/:type/:id/status`
- `POST /api/v1/admin/op/exceptions/:type/:id/notes`
- `GET /api/v1/admin/op/users/:id/summary`
- `GET /api/v1/admin/op/accounts/health`

The first implementation can also be frontend-composed from existing APIs if performance is acceptable. Backend snapshots should be added when a page requires many calls or expensive joins.

## Frontend Direction

Keep the OP workspace in the existing frontend:

- Add routes under `frontend/src/router/index.ts`.
- Add OP API wrappers under `frontend/src/api/admin/op.ts` only for new backend endpoints.
- Add views under `frontend/src/views/admin/op/`.
- Reuse existing stores, i18n, layout, table, chart, and modal patterns.
- Link to existing admin detail pages for advanced edit flows.

UI should be dense and operational, not marketing-style:

- KPI strip for current state
- Queue tables with filters and saved views
- Detail drawers for quick triage
- Safe action buttons with confirmation and reason fields
- Deep links to existing user/account/order/request pages

## Data and Performance Notes

- Prefer existing aggregated usage/dashboard tables for time-range summaries.
- Avoid scanning raw usage logs for default OP pages.
- Use snapshot endpoints for heavy overview pages.
- Keep exception queue filters indexed by status, severity, created time, user, account, and order where applicable.
- Do not expose secrets in OP DTOs.

## Open Questions

- Should OP users be separate from admins, or should all current admins see the OP workspace initially?
- Which role should be allowed to adjust user balance manually?
- Do payment refunds need to be executed through provider APIs or only recorded as manual markers at first?
- Should operator notes be generic across entities, or only attached to exception queue items in the MVP?
- Should the initial exception queue unify data only in the frontend, or create a backend-normalized queue DTO immediately?

## Suggested Implementation Order

1. Add OP route shell and overview page using existing APIs.
2. Add exception queue read-only view, backed by existing alert/error/risk/payment APIs.
3. Add User 360 read-only summary.
4. Add narrow quick actions with reason fields.
5. Add audit log storage and enforce audit writes for sensitive OP actions.
6. Add RBAC and split operator/finance/on-call/marketing permissions.
7. Expand campaign and revenue operations.
