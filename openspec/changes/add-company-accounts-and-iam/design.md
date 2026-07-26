## Context

Sub2API currently models every login as an independent user. `users` owns email authentication, balance, frozen balance, API keys, subscriptions, and usage; authorization is primarily the global `admin`/`user` role. Billing code assumes the consuming user is also the payer, and usage records do not retain an organization or payer snapshot.

This change adds a second authorization boundary without granting system-admin access: an approved company root owns one organization, creates IAM identities, assigns system-managed policies, allocates funds, and views company-scoped usage. It also changes the meaning of billing for IAM users, so authentication, gateway billing, balance RPC, asynchronous media settlement, usage, email delivery, and frontend navigation must move together.

Alibaba Cloud's private account-ID generation algorithm is not public and cannot be reproduced from example IDs. The compatible contract we can implement is its visible identity hierarchy: one 16-digit root account ID shared as the IAM account namespace, an independent 18-digit IAM user ID, and an IAM login principal composed as `<login_name>@<root-account-id>.opentk.ai`.

Project constraints:

- PostgreSQL SQL migrations are the production schema source; Ent schemas must remain in parity.
- Existing internal numeric primary keys remain the relational keys. Public decimal identifiers are external identifiers, not replacements for primary keys.
- Existing personal email login and balances must remain compatible.
- New money movements and review decisions need atomicity, idempotency, and immutable audit evidence.
- An organization owner is not a system administrator and must never inherit `/admin/*` access.
- The company feature must remain disabled until all billable paths use the same payer-resolution contract.

## Goals / Non-Goals

**Goals:**

- Add immutable, string-safe public account identifiers and a safe rollout for all legacy users.
- Add a paid USD 20-by-default company upgrade application with freeze, capture, release, review, and durable email notification.
- Add a single-owner organization boundary and a default limit of 20 non-archived IAM users.
- Add unambiguous IAM principal/password login, one-time initial credentials, forced password change, and member lifecycle controls.
- Add Alibaba RAM-inspired system-managed policies with deny-by-default action checks and immediate cache invalidation.
- Support allocated balance and shared root balance while preserving the original payer through holds, settlement, refund, and reconciliation.
- Add organization-scoped usage and finance views that cannot expose system-admin-only fields or another organization's data.
- Preserve immutable financial, authorization, membership, and review history.

**Non-Goals:**

- Reproduce or claim Alibaba Cloud's undisclosed random-number algorithm.
- Perform legal entity verification, business-license validation, or automatic company-name uniqueness enforcement.
- Import an existing personal user into an organization.
- Support multiple organization administrators, owner transfer, custom policies, policy groups, explicit deny policies, conditional policies, or resource-level policy documents in phase one.
- Support company downgrade, dissolution, or reuse of archived IAM identities.
- Allow IAM users to recharge, redeem value, purchase subscriptions, or transfer affiliate value.
- Include personal usage from before company approval in company reports.

## Decisions

### 1. Use an Alibaba-compatible identity hierarchy with project-owned CSPRNG generation

Public identity fields are strings:

| Identity | `account_id` | `external_user_id` | Login |
| --- | --- | --- | --- |
| Personal root | independent 16 digits | same 16 digits | existing email login |
| Company root | existing root 16 digits | same 16 digits | existing email login |
| IAM user | company root's 16 digits | independent 18 digits | `<login_name>@<account_id>.opentk.ai` + password |

Both generators use rejection sampling over a cryptographically secure byte stream: select the first decimal digit uniformly from `1..9`, then each remaining digit uniformly from `0..9`. Go uses `crypto/rand`; the operator backfill uses Python `secrets`. A database uniqueness conflict causes bounded regeneration. IDs contain no timestamp, shard, primary key, account type bit, or check digit.

The 16-digit root namespace has `9 * 10^15` candidates and the 18-digit IAM namespace has `9 * 10^17`. Random generation does not eliminate collision probability, so the global database uniqueness constraint remains authoritative. IDs are never changed or returned to the pool after soft deletion.

**Alternative: Snowflake/time-based decimal IDs.** Rejected because timestamp and worker layout are predictable, operationally couple uniqueness to worker configuration, and do not match the opaque external-ID behavior requested.

**Alternative: derive IAM IDs from the root ID or login name.** Rejected because it leaks relationships, makes rename semantics difficult, and reduces the independent namespace.

### 2. Keep internal keys and add explicit identity and organization tables

The additive schema is:

- `users`: add nullable-at-first `account_id`, `external_user_id`, `identity_type` (`root|iam`), normalized IAM `login_name`, `must_change_password`, `recovery_email`, `recovery_email_verified_at`, and an authorization generation. `email` becomes nullable for IAM rows; personal/root login continues to require it. Use a global unique index on populated `external_user_id` and a partial case-insensitive unique index on `(account_id, lower(login_name))` for non-archived IAM users.
- `organizations`: internal ID, unique root `account_id`, owner user ID, current name, normalized name for similarity search, `active|suspended` status, approval/effective time, and member limit defaulting to 20.
- `organization_memberships`: unique user membership, organization, `owner|member` role, lifecycle status, authorization generation, and archive metadata. Approval creates exactly one owner membership; phase-one constraints prevent a second owner.
- `company_upgrade_applications`: applicant, requested name, status, fee amount/currency snapshot, reservation idempotency key, reviewer/reason/timestamps, and ledger references.
- `organization_name_change_requests`: organization, old/new name snapshots, review state, reviewer/reason/timestamps.
- `managed_policies` and `managed_policy_actions`: immutable policy key, type, display metadata, version, and action strings. Seed phase-one rows idempotently.
- `member_policy_attachments`: organization/member/policy version, actor, attached time, and detached time. A partial unique index prevents duplicate active attachments.
- `organization_financial_ledger`: append-only `upgrade_reserve|upgrade_capture|upgrade_release|allocate|reclaim` movements with idempotency key, source/destination, amount/currency, actor, organization/application, and balance snapshots.
- `organization_audit_events`: append-only domain audit for review, membership, policy, suspension, and financial commands. Existing generic admin audit logging remains in place for HTTP-level system-admin evidence.
- `notification_outbox`: deduplication key, event, recipient, locale, template variables, retry state, next-attempt time, and terminal delivery result.

`usage_logs`, `async_media_tasks`, `batch_image_jobs`, and `balance_ledger` gain nullable `organization_id` and `payer_user_id` snapshots; billing tables also retain balance source and consuming user where not already present. New rows populate these fields, while historical nulls remain valid.

Making `users.email` nullable is preferable to generating fake IAM email addresses. Fake addresses would pollute notifications and profile APIs and could accidentally become authentication identifiers. Services and DTOs that currently assume non-empty email must branch on `identity_type`; IAM recovery uses its separate optional verified field.

**Alternative: replace numeric user primary keys with public account IDs.** Rejected because it would rewrite nearly every foreign key and hot query without product benefit.

### 3. Roll out identifiers in two schema phases

The first migration adds nullable fields, format checks that apply only when values are non-null, and a partial global unique index on `external_user_id`. New root creation dual-writes IDs immediately. A standalone script in `backend/tools/`:

1. supports `--dry-run`, batch size, start-after cursor, and bounded retries;
2. locks a bounded set of rows, skips populated users, and commits each batch;
3. assigns every legacy user root semantics (`account_id == external_user_id`, 16 digits);
4. reports scanned, populated, skipped, collision-retried, and failed counts without printing sensitive user data.

After operators verify zero null/invalid rows, a second migration replaces the partial index with a global unique constraint, makes both identifiers non-null, and enforces identity-specific length/equality checks. IAM creation is feature-gated until the second migration is complete.

**Alternative: assign IDs in one blocking migration.** Rejected because a large users table would create an uncontrolled migration transaction and make operational retries difficult.

### 4. Separate IAM login from personal login

Add `/api/v1/auth/iam/login` and an IAM mode on the existing login view. The UI collects the complete `<login_name>@<account_id>.opentk.ai` principal and password, and the backend parses the principal to recover the normalized login name and 16-digit account ID. This avoids a separate account-ID field and keeps the canonical identity self-contained.

The IAM login query uses `(account_id, lower(login_name))`, then verifies organization and membership status before password verification. It returns the existing token-pair shape with additional identity, organization, authorization-generation, and `must_change_password` claims. Member creation accepts an owner-entered password and the frontend offers a Web Crypto-based generator; `must_change_password` defaults to true but the owner may clear it. Owner-reset passwords continue to use `crypto/rand` and require a password change. Password plaintext is returned only by the create/reset command and is never persisted or logged.

All IAM API-key authentication resolves current membership and authorization generation, rather than trusting policy claims captured when the key was created. IAM recovery email is optional and must be verified before self-service recovery; otherwise the owner resets the password.

**Alternative: send IAM users through the email login endpoint.** Rejected because IAM users need no email, recovery email is not an identity, and heuristic parsing creates ambiguous credentials.

### 5. Treat company approval and fee settlement as one state machine

Application states are `pending`, `approved`, `rejected`, and `withdrawn`. The service uses PostgreSQL transactions and row locks:

```text
submit:   available - fee, frozen + fee, reserve ledger, pending application
approve:  frozen - fee, capture ledger, organization + owner membership, approved
reject:   frozen - fee, available + fee, release ledger, rejected + reason
withdraw: frozen - fee, available + fee, release ledger, withdrawn
```

The amount and currency are read from configuration only on submission and stored on the application. All later transitions use that snapshot. Unique idempotency keys and a compare-on-`pending` update guarantee one terminal decision. Any active system administrator may review an application, including their own, and the reviewer remains recorded in the immutable audit trail. Approved captures are non-refundable.

Company names are normalized for review search and trigram similarity warnings but are not unique. Company name changes use their own review request and do not reserve another upgrade fee. Suspension is a reversible system-admin operation; it does not delete identities or financial history.

**Alternative: deduct USD 20 only after approval.** Rejected because balance could disappear during review and approval would then fail after an administrator had accepted the application.

**Alternative: deduct immediately and refund on rejection.** Rejected because a frozen reservation expresses pending ownership clearly and prevents the amount being spent twice.

### 6. Use system-managed action policies and an implicit owner role

Phase-one action keys are:

- `organization.finance.balance.read`, seeded as `CompanyFinanceReadOnly`.
- `organization.balance.shared.use`, seeded as `CompanySharedBalanceUse`.

Owner management actions are checked from the `owner` membership role and cannot be attached to IAM users. Policy evaluation is deny-by-default and receives a trusted organization context from middleware. It never accepts organization scope from a request body.

Authorization caches are keyed by user plus authorization generation. Attaching/detaching a policy, disabling/archiving a member, suspending an organization, or updating a managed policy increments the affected generation in the database and synchronously invalidates Redis and local session/API-key caches before the mutation response returns. When invalidation infrastructure is unhealthy, organization privileged actions and IAM API-key checks fail closed to a database read until caches converge.

This versioned action model supports adding managed policies later without introducing a general JSON policy language now.

**Alternative: reuse the global `role` field.** Rejected because it cannot represent organization scope and risks granting system-admin routes.

**Alternative: clone Alibaba RAM's full policy document language in phase one.** Rejected because two fixed permissions do not justify wildcard resources, conditions, explicit deny precedence, groups, and custom-policy lifecycle yet.

### 7. Resolve and snapshot the effective payer once

Introduce a `BillingContextResolver` used by gateway preflight, usage billing, balance RPC, asynchronous media, batch image, reconciliation, refunds, cache invalidation, and low-balance notification:

```go
type BillingContext struct {
    ConsumerUserID int64
    OrganizationID *int64
    PayerUserID    int64
    BalanceSource  string // self, allocated, shared
    AuthzGeneration int64
}
```

Resolution rules:

```text
personal/root -> payer = consumer, source = self
IAM + shared action -> payer = root, source = shared
IAM without shared action -> payer = consumer, source = allocated
```

Once a hold or deduction starts, the context is persisted and later stages must not resolve it again. Insufficient funds fail against the selected payer with no fallback. Refund and release always use the original snapshot. Low-balance notifications and balance-cache updates follow the payer. Usage preserves both consumer and payer.

New financial code uses PostgreSQL `numeric` and the existing `shopspring/decimal` dependency for application-side validation and serialization. It does not introduce new float arithmetic for ledger transitions.

**Alternative: check shared permission independently in every billing path.** Rejected because permission changes between reserve and refund would send money to the wrong wallet and implementations would drift.

### 8. Keep allocation separate from shared balance

Allocation transfers available funds from root to IAM-owned available balance. Reclaim transfers only the member's available, unfrozen amount back. Both lock source and destination rows in stable ID order, write paired ledger movements, and use a command idempotency key.

Shared balance does not transfer funds. It only changes the payer chosen for a request. Revoking shared permission therefore makes the next request use any existing allocated member balance. Neither finance-read nor shared-use permission enables recharge or subscription operations.

### 9. Derive organization usage scope in the backend

Organization handlers live under `/api/v1/organization/*`. Middleware resolves the active owner membership and places the internal organization ID in context. Repository queries always include this ID and the organization's effective timestamp; a member filter is joined back to the same organization.

Usage DTOs reuse user-visible fields from the existing usage view but use a dedicated organization mapper that omits upstream account, internal cost/profit, raw error, secrets, and admin-only diagnostics. Composite indexes beginning with `(organization_id, created_at)` support list and aggregation queries on partitioned usage data.

The frontend adds focused organization pages under `frontend/src/views/organization/`, typed APIs, route metadata, and sidebar predicates. It reuses the existing usage table/filters where safe, but does not reuse `/admin/*` handlers or DTOs. Account-menu data comes from the current-user response so navigation does not infer permissions locally.

### 10. Use a durable notification outbox and existing mail templates

Review transactions insert business state and `notification_outbox` rows together. A worker claims rows with `FOR UPDATE SKIP LOCKED`, renders registered `NotificationEmailService` events, retries with bounded exponential backoff, and records delivery outcome. A logical deduplication key such as `<event>:<application>:<recipient>` prevents duplicate sends during retries.

The current in-memory `EmailQueueService` is not sufficient for approval notifications because process restart can lose queued work. It remains available for existing flows; the outbox worker invokes the existing template and SMTP abstraction.

## Risks / Trade-offs

- **[Billing path omitted]** An IAM request could charge or refund the wrong wallet. → Gate organization activation behind a billing-path inventory and tests covering gateway, RPC, async media, batch image, cache, notifications, and reconciliation.
- **[Authorization cache race]** Revoked permissions could remain usable briefly on another instance. → Use generation-based keys, synchronous Redis invalidation, local broadcast, and fail-closed database reads when invalidation is unhealthy.
- **[Nullable email compatibility]** Existing code may assume every user has an email. → Add identity-aware DTO/service tests and audit all email, profile, login, notification, and admin-user paths before enabling IAM creation.
- **[Identifier collision or weak implementation]** Decimal namespaces are finite and a modulo implementation can bias output. → Centralize rejection-sampling generators, keep the database constraint authoritative, retry boundedly, and test format/concurrency rather than deterministic values.
- **[Concurrent member creation]** Count-then-insert can exceed 20. → Serialize creation on the organization row and enforce the check inside the insert transaction.
- **[Financial deadlock]** Allocation/reclaim locks two user balances. → Lock user rows in ascending internal-ID order and keep transactions small.
- **[Usage query cost]** Organization-wide reports broaden the current user-scoped scan. → Snapshot indexed organization IDs, preserve time bounds, paginate, and add query-plan/integration tests on partitions.
- **[Email duplication]** Worker retries can send the same message twice after an uncertain provider response. → Use logical deduplication and delivery records; accept that SMTP cannot provide exactly-once delivery, while guaranteeing at-least-once processing.
- **[Old binary rollback]** Once IAM rows with null personal email exist, older binaries may not understand them. → Keep IAM creation behind a feature flag and treat enablement as a forward-only compatibility point; rollback disables logins/creation without deleting data.
- **[Name similarity quality]** Trigram warnings can produce false positives. → Keep them advisory and leave the final decision to system administrators.

## Migration Plan

1. Add nullable public-ID fields and partial uniqueness/format checks; deploy dual-write generation for all newly created root users.
2. Run the Python backfill in dry-run mode, then in bounded batches; verify no missing, malformed, or duplicate IDs.
3. Apply the final ID constraint migration and enable ID display. Do not enable IAM creation before this point.
4. Add organization, application, membership, policy, attachment, financial ledger, audit, and notification-outbox schemas plus Ent parity; seed managed policies idempotently.
5. Deploy IAM authentication, organization authorization middleware, lifecycle services, outbox worker, and admin review APIs behind disabled feature flags.
6. Add payer snapshots and the central resolver to every billing path. Run reconciliation and end-to-end tests for allocated/shared funds and permission changes during asynchronous work.
7. Deploy organization and review frontend routes, then enable company applications. Existing users remain personal until approved.
8. Monitor application reservations versus ledger totals, orphaned frozen fees, organization member counts, payer/consumer snapshot completeness, outbox lag, and authorization fallback rates.

Rollback before company activation disables the feature flags and leaves additive columns/tables unused. After applications exist, rollback SHALL release only still-pending reservations through the normal idempotent withdrawal/rejection command; it SHALL NOT delete ledger rows or reverse approved fees. After IAM users exist, rollback disables IAM login and mutations but retains identities and history until a compatible forward fix is deployed.

## Open Questions

None for phase one. Product defaults, including the USD 20 upgrade fee, 20-member IAM limit, sole-owner model, policy behavior, notification recipients, and lifecycle restrictions, are approved; both values remain configuration-backed for later operational changes.
