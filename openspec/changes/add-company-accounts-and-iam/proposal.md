## Why

Sub2API currently treats every user as an independent personal account with an email login, a directly owned balance, and only the system-level `admin`/`user` role split. Companies need an approved organization boundary, Alibaba Cloud RAM-style member identities and managed permissions, auditable balance sharing or allocation, and company-scoped usage visibility without granting system administrator access.

## What Changes

- Add immutable public account and user identifiers: a 16-digit company/root account ID and an independently generated 18-digit IAM user ID, both backed by cryptographically secure generation, database uniqueness, and a resumable legacy-user backfill.
- Add a company-account upgrade request from the account menu. The request captures the company name, reserves a configurable USD 20 fee, notifies system administrators, and becomes effective after a system administrator approves it; an eligible system administrator may review their own request.
- On approval, create the organization atomically, make the applicant its sole owner administrator, and settle the reserved fee; rejection or withdrawal releases the reservation and notifies the applicant.
- Add IAM member creation and lifecycle management with a default organization limit of 20 non-archived IAM users, owner-supplied or securely generated initial passwords, configurable first-login password changes enabled by default, optional verified recovery email, and login principals in the form `<login-name>@<root-account-id>.opentk.ai`.
- Add Alibaba RAM-inspired system-managed policy attachments. Phase one provides finance read-only and shared-balance consumption policies while reserving a versioned action model for future policies.
- Add organization balance allocation and reclaim operations with an immutable ledger. IAM users cannot recharge, redeem value, purchase subscriptions, or transfer affiliate value.
- Resolve the actual payer consistently across gateway billing, balance RPC, and asynchronous media hold/capture/release flows. Shared-balance permission always selects the root account; otherwise the IAM user's allocated balance is used, with no automatic fallback.
- Snapshot organization and payer identifiers on usage records and provide company-scoped usage APIs and console pages that expose member consumption without exposing upstream accounts, internal account cost, raw errors, or system-admin functionality.
- Extend the current-user and account-menu payloads to show public account ID, company, organization role, IAM login name, permissions, and non-sensitive balance-source information.
- Add durable, retryable notification delivery and append-only audit records for upgrade review, member lifecycle, policy changes, and organization fund movements.

## Capabilities

### New Capabilities

- `public-account-identifiers`: Immutable root account IDs and IAM user IDs, secure generation, string-safe API representation, uniqueness, and phased legacy backfill.
- `company-account-upgrade`: Paid company upgrade application, USD 20 reservation and settlement, system-admin review, organization activation, rejection, and email notification.
- `iam-members-and-auth`: Organization member creation and lifecycle, 20-member default limit, IAM login principals, initial credentials, password recovery, and IAM restrictions.
- `organization-authorization`: Organization roles, system-managed policies, direct member attachments, permission evaluation, cache invalidation, and authorization audit.
- `organization-billing`: Balance allocation and reclaim, shared-balance payer resolution, payment-source snapshots, IAM purchase restrictions, and organization financial visibility.
- `organization-usage-and-console`: Organization-scoped usage records, safe company usage reports, organization routes, conditional console navigation, and account-menu identity information.

### Modified Capabilities

- `balance-rpc`: Resolve and audit the effective payer for IAM users, keep refunds bound to the original payer, and return balance information without leaking root balance to unauthorized IAM identities.
- `media-prepay-billing`: Reserve, capture, and release asynchronous media funds against the snapshotted effective payer rather than assuming the consuming user owns the charged balance.

## Impact

- **Database:** new organization, upgrade request, membership, managed policy, attachment, balance transfer, notification outbox, and organization audit data; new user identity fields; payer and organization snapshots on usage and billing records.
- **Backend:** authentication DTOs and IAM login, user creation, organization services and middleware, system-admin review APIs, payment and redeem guards, payer resolution, gateway/API-key caches, balance RPC, async media settlement, usage queries, email templates/outbox, and audit actions.
- **Frontend:** account menu, company upgrade dialog/status, admin review queue, organization member and authorization pages, organization usage view, finance balance view, IAM login mode, route guards, sidebar visibility, and i18n.
- **Operations:** two-stage public-ID rollout with a separately run Python backfill and a follow-up constraint migration; configurable upgrade fee and IAM-member limit; new indexes, metrics, reconciliation checks, and notification worker health.
- **Compatibility:** internal numeric user primary keys remain unchanged and existing personal accounts continue to operate. Public identifiers are JSON strings. Company functionality remains unavailable to legacy rows until their public IDs are backfilled.
