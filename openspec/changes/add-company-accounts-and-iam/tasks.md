## 1. Public Account Identifier Foundation

- [x] 1.1 Add the phase-one SQL migration and matching Ent user fields for nullable `account_id`, `external_user_id`, `identity_type`, IAM login metadata, nullable IAM email handling, format checks, and the partial global unique index.
- [x] 1.2 Implement centralized 16-digit root and 18-digit IAM CSPRNG generators using unbiased rejection sampling, bounded uniqueness retries, and unit/concurrency tests.
- [x] 1.3 Dual-write root identifiers in registration, OAuth provisioning, setup-admin creation, test fixtures, and every other user-creation path while preserving existing email login behavior.
- [x] 1.4 Add `backend/tools/` Python backfill with `secrets`, dry-run, batch/cursor resume, collision retry, safe reporting, and script tests against duplicate and partially populated data.
- [x] 1.5 Add an operator verification query/command and the phase-two SQL migration that refuses incomplete data, makes IDs required, installs the full global unique constraint, and enforces root/IAM length and equality semantics.
- [x] 1.6 Extend current-user, admin-user, and profile DTO mappers to serialize public IDs as strings and add regression tests proving no JSON numeric coercion or identifier mutation.

## 2. Organization Persistence And Configuration

- [x] 2.1 Add SQL migrations and Ent parity for organizations, memberships, company-upgrade applications, company-name-change requests, managed policies/actions, policy attachments, organization financial ledger, organization audit events, and notification outbox.
- [x] 2.2 Add nullable organization/payer/balance-source snapshots and supporting indexes to usage logs, balance ledger, asynchronous media tasks, and batch image jobs without rewriting historical rows.
- [x] 2.3 Add validated configuration for company features, USD 20 default upgrade fee/currency, 20-member default limit, outbox retry timing, and rollout gates; keep company application/IAM creation disabled until prerequisites are complete.
- [x] 2.4 Seed `CompanyFinanceReadOnly` and `CompanySharedBalanceUse` with stable keys, versions, descriptions, and action rows idempotently in migration/setup tests.
- [x] 2.5 Implement organization repositories with same-organization predicates, row-lock helpers, normalized-name similarity queries, and integration tests for unique owner and active-attachment constraints.

## 3. Paid Company Upgrade Workflow

- [x] 3.1 Implement eligibility and application-query services for personal roots, including one-pending-application enforcement and duplicate/similar company-name warnings.
- [x] 3.2 Implement idempotent application submission that snapshots the configured fee and atomically moves it from available to frozen balance with an immutable reserve ledger entry.
- [x] 3.3 Implement applicant withdrawal that atomically transitions only pending applications and releases the exact snapshotted fee once.
- [x] 3.4 Implement system-admin review list/detail APIs with applicant, company-name warning, fee, state, and audit information while excluding secrets.
- [x] 3.5 Implement approval for any active system administrator, including self-review, with row locking, frozen-fee capture, organization activation, sole-owner membership creation, effective timestamp, and duplicate/concurrent-decision tests.
- [x] 3.6 Implement rejection with mandatory reason, exact frozen-fee release, reviewer metadata, idempotent terminal-state handling, and insufficient/inconsistent-frozen-balance reconciliation tests.
- [x] 3.7 Implement company-name change request/review and system-admin organization suspend/reactivate commands with retained history and IAM access enforcement.
- [x] 3.8 Add user `/organization/applications` and admin review routes, authorization middleware, request validation, domain audit actions, and handler/service tests for every state transition.

## 4. Durable Review Notifications

- [x] 4.1 Implement notification-outbox repository claiming with `FOR UPDATE SKIP LOCKED`, logical deduplication, bounded exponential retry, delivery state, and lag metrics.
- [x] 4.2 Register localized notification templates/events for upgrade submission, approval, rejection, withdrawal, and company-name decisions using the existing `NotificationEmailService` renderer/sender.
- [x] 4.3 Insert recipient-specific outbox rows in the same transaction as each application/name-change transition, including all eligible system administrators on submission and the applicant on terminal outcomes.
- [x] 4.4 Wire the outbox worker into server lifecycle and add tests for restart recovery, provider failure, concurrent workers, logical deduplication, and business-transaction independence from delivery.

## 5. IAM Member Lifecycle And Authentication

- [x] 5.1 Implement owner-only member creation with login-name validation/normalization, canonical principal construction, independent 18-digit ID generation, zero balance/grants, and trusted organization scoping.
- [x] 5.2 Enforce the configurable 20 non-archived member limit inside a serialized organization transaction and test active/disabled/archive accounting plus concurrent twentieth-member creation.
- [x] 5.3 Generate a secure initial password, persist only its hash, return plaintext exactly once, and add redaction tests covering logs, audit events, subsequent reads, and error responses.
- [x] 5.4 Add `/auth/iam/login` principal/password authentication with organization/member status checks, generic failure responses, existing token-pair compatibility, and no recovery-email login.
- [x] 5.5 Enforce restricted first-login sessions until password change, including middleware allow-list tests for password-change and logout only.
- [x] 5.6 Implement optional recovery-email verification and self-service reset; implement owner reset for members without verified recovery, with forced password change and session revocation.
- [x] 5.7 Implement owner-only list/detail/disable/enable/archive member APIs, immediate session/API-key invalidation, immutable archived history, and cross-organization non-disclosure tests.
- [x] 5.8 Audit and update user/email assumptions across profile, notification, admin-user, OAuth, TOTP, API-key, and test-fixture code so nullable IAM email cannot panic or become a login identifier.
- [x] 5.9 Add backend guards that deny IAM recharge, payment-order creation, redemption, subscription purchase, and affiliate-value transfer even when endpoints are called directly.

## 6. Organization Authorization

- [x] 6.1 Implement organization context middleware that derives membership and organization from the authenticated user/API key and never trusts body/query organization scope.
- [x] 6.2 Implement deny-by-default action evaluation for implicit owner actions and direct versioned managed-policy attachments.
- [x] 6.3 Add owner-only policy list, member-attachment list, attach, and detach APIs with same-organization enforcement and immutable authorization audit events.
- [x] 6.4 Add authorization-generation increments and synchronous Redis/local session/API-key cache invalidation for policy, membership, organization status, and managed-policy version changes.
- [x] 6.5 Implement fail-closed database fallback when authorization invalidation is unhealthy and test permission revocation against already-issued sessions and API keys across cache instances.
- [x] 6.6 Add explicit tests proving organization-owner capabilities do not grant global `admin` role/routes and IAM users cannot receive the owner role in phase one.

## 7. Allocation And Effective-Payer Core

- [x] 7.1 Implement `BillingContextResolver` for personal/root self-pay, IAM allocated balance, and IAM shared root balance with organization/member/policy status and authorization-generation snapshots.
- [x] 7.2 Implement owner-only allocation and reclaim commands using decimal arithmetic, stable two-user lock ordering, immutable paired ledger movements, non-overdraft checks, and command idempotency.
- [x] 7.3 Add finance-summary APIs that expose root balances only to owner/`CompanyFinanceReadOnly`, expose member balances to that member, and return only non-numeric shared-source status to shared-only users.
- [x] 7.4 Add integration tests proving shared permission never transfers funds, revocation restores allocated-balance selection, and insufficient selected-payer balance never falls back.

## 8. Billing-Path Integration

- [x] 8.1 Extend gateway preflight and synchronous usage billing to resolve once, deduct/cache/notify the effective payer, and snapshot consumer, organization, payer, source, and authorization generation on usage.
- [x] 8.2 Extend balance RPC protobufs, service, repository, and responses so deductions snapshot consumer/payer context, refunds use the original payer, replay preserves the original result, and public mappings cannot leak root amounts.
- [x] 8.3 Extend asynchronous media reservation, capture, release, terminal usage, and reconciliation to persist and reuse the original payer snapshot across permission/lifecycle changes.
- [x] 8.4 Extend batch-image reservation, worker settlement, cancellation, refund, recovery, and terminal usage paths to persist and reuse the original payer snapshot.
- [x] 8.5 Update all balance-cache invalidation and low-balance notification paths to target the effective/original payer rather than the consumer.
- [x] 8.6 Inventory remaining deduction, refund, compensation, and reconciliation call sites with `rg`; add a checked-in billing-path matrix and tests or explicit non-billable rationale for every path before enabling company activation.
- [x] 8.7 Add concurrency/integration tests for policy revocation after hold, member disable/archive after deduction, organization suspension, duplicate settlement, partial refund, and root/member balance conservation.

## 9. Organization Usage And Backend APIs

- [x] 9.1 Populate `organization_id` and `payer_user_id` on every new usage-log producer and add tests that historical nullable records remain readable.
- [x] 9.2 Add organization-scoped usage repository queries and aggregations with mandatory organization/effective-time predicates, time bounds, pagination, and member/API-key/model/endpoint/status filters.
- [x] 9.3 Add `/api/v1/organization/usage` list/stats/trend endpoints and dedicated company-visible DTO mappers that omit upstream account, internal cost/profit, raw errors, secrets, and admin-only metadata.
- [x] 9.4 Add query-plan and partition integration tests for `(organization_id, created_at)` indexes and cross-organization filter attacks.
- [x] 9.5 Extend the current-user endpoint with company, membership role/status, IAM principal/user ID, effective policy names/actions, first-login state, and redacted balance-source fields.

## 10. Frontend Company And IAM Experience

- [x] 10.1 Add TypeScript organization/auth models and API clients for applications, IAM login, members, policies, allocation, finance, usage, and admin review.
- [x] 10.2 Add a personal-account menu upgrade action and application modal/status view showing company name, current snapshotted fee, insufficient-balance errors, pending state, and withdrawal.
- [x] 10.3 Add system-admin review navigation and list/detail decision UI with similar-name warnings, self-review support, mandatory rejection reason, fee snapshot, and terminal state.
- [x] 10.4 Add IAM login mode with complete canonical-principal/password inputs, backend principal parsing, generic authentication errors, and forced first-password-change flow.
- [x] 10.5 Extend the account menu and profile to display account ID, main/sub-account identity, IAM user ID/principal, company, role/status, effective policy names, and authorized non-sensitive balance-source information.
- [x] 10.6 Add owner member management with stable 20-slot status, one-time initial/reset credential display, disable/enable/archive controls, and responsive validation states.
- [x] 10.7 Add owner authorization and balance-allocation views using policy descriptions, explicit attach/detach state, member allocated balance, and reclaim limits.
- [x] 10.8 Add company usage and finance-read views by reusing user-safe usage components while excluding all admin-only columns and filters.
- [x] 10.9 Add organization route metadata/sidebar predicates and direct-navigation guards for personal, owner, IAM-policy, first-login-restricted, and suspended states.
- [x] 10.10 Add Chinese/English locale strings and focused component/store/router/API tests for menu visibility, IAM restrictions, approval states, redaction, and mobile/desktop text containment.
- [x] 10.11 Add owner-entered or securely generated IAM initial passwords, a default-on first-login password-change option, and single-field canonical `<login_name>@<account_id>.opentk.ai` principals across creation, display, and authentication.

## 11. Audit, Reconciliation, And Rollout Verification

- [x] 11.1 Add immutable domain audit coverage and correlation IDs for application review, name changes, organization suspension, member lifecycle, policy changes, allocation/reclaim, and failed cross-organization attempts.
- [x] 11.2 Add reconciliation jobs/queries for pending application reservations, frozen-fee/ledger totals, owner cardinality, member-limit violations, financial transfer conservation, and missing payer snapshots after enablement.
- [x] 11.3 Add metrics and alerts for ID collision retries, review queue age, outbox lag/failures, authorization database fallbacks, denied IAM financial operations, and payer-resolution failures.
- [x] 11.4 Run migration integration tests through both ID phases and document the exact dry-run, backfill, verification, constraint, policy-seed, feature-enable, and rollback procedure.
- [x] 11.5 Run backend unit tests with `go test -tags=unit ./...` and integration tests with `go test -tags=integration ./...` from `backend`, fixing all failures.
- [x] 11.6 Run `govulncheck ./...` and `golangci-lint run` from `backend`, fixing or documenting any external advisory that cannot be remediated in scope.
- [x] 11.7 Run `make test-frontend` and `eslint . --ext .vue,.js,.jsx,.cjs,.mjs,.ts,.tsx,.cts,.mts` from the repository root, fixing all failures.
- [x] 11.8 Generate `frontend/audit.json`, run `python tools/check_pnpm_audit_exceptions.py --audit frontend/audit.json --exceptions .github/audit-exceptions.yml` with the repository's expected working directory, and resolve unapproved findings.
- [x] 11.9 Perform end-to-end acceptance for upgrade reserve/approve/reject/withdraw emails, 20-member concurrency, IAM login/recovery, policy revocation, allocation/shared billing, async refund to original payer, finance redaction, usage scoping, and organization suspension.
- [x] 11.10 Keep company application and IAM creation feature flags disabled until backfill constraints, billing-path matrix, reconciliation checks, and acceptance tests all pass; then enable in staged rollout with rollback monitoring.
