# Auth Identity And Payment Foundation Design

**Date:** 2026-04-20

**Status:** Draft approved in conversation, written for implementation planning

**Goal**

Rebuild the `feat/auth-identity-foundation` intent on a clean branch from `main`, covering unified user identity, third-party login and binding, profile adoption, source-based signup defaults, unified payment routing and UX, admin configuration, compatibility with existing `main` data, and an opt-in OpenAI advanced scheduling switch.

## Scope

This design includes:

- Email login and registration
- Third-party login and binding for `LinuxDo`, `OIDC`, and `WeChat`
- Unified identity storage for email and third-party identities
- Pending auth sessions for callback-to-login/register/bind continuation
- User-controlled nickname/avatar adoption during first relevant third-party flow
- Profile binding management and avatar upload/delete
- Source-based initial grants for balance, concurrency, and subscriptions
- User management support for `last_login_at` and `last_active_at` sorting
- Unified payment display methods (`alipay`, `wechat`) mapped to a single active backend source each
- Alipay and WeChat UX routing rules across PC, mobile, H5, and WeChat environments
- Admin settings for auth providers, source defaults, payment sources, and OpenAI advanced scheduling
- Incremental migration and compatibility for existing email users and historical LinuxDo synthetic-email users

This design does not treat unrelated upstream merges, docs churn, or license changes from the old branch as required scope.

## Product Rules

### Auth and identity

- Existing email users remain valid and continue to log in with no manual action.
- Third-party first login behavior:
  - Existing bound identity: direct login
  - Missing identity: start first-login flow
- If `force_email_on_third_party_signup` is disabled, a first-login user may create an account without binding an email.
- If `force_email_on_third_party_signup` is enabled, the user must provide an email.
- If the provided and verified email already exists:
  - show that the email already exists
  - allow "verify and bind existing account"
  - allow "change email and continue registration"
  - do not allow bypassing the email requirement
- Upstream provider email verification is not trusted as a local bound email.
- WeChat login chooses channel by environment:
  - in WeChat environment: `mp`
  - outside WeChat: `open`
- WeChat primary identity key is `unionid`.
- If a WeChat login/bind flow cannot produce `unionid`, the flow fails and no fallback `openid` identity is created.

### Profile adoption

- During the first relevant third-party flow, the user can independently decide:
  - replace current nickname or not
  - replace current avatar or not
- This applies to first third-party registration and first third-party binding.
- The decision is explicit user choice, not automatic replacement.

### Source-based initial grants

- Source-specific defaults exist for `email`, `linuxdo`, `oidc`, and `wechat`.
- Each source defines:
  - default balance
  - default concurrency
  - default subscriptions
  - grant on signup
  - grant on first bind
- Default behavior:
  - grant on signup: enabled
  - grant on first bind: disabled
- First-bind grants are optional and controlled per source.
- Grants must be idempotent.

### Avatar management

- Avatar supports:
  - external URL
  - image `data:` URL
- `data:` URL images are compressed to at most `100KB` before persistence.
- Avatar storage is database-backed.
- Avatar delete is supported.

### Payment UX and routing

- Frontend shows only two display methods:
  - `alipay`
  - `wechat`
- Users never choose between official providers and EasyPay explicitly.
- Backend allows only one active source per display method at a time.
- Alipay UX:
  - PC: show QR code in page
  - mobile: jump to Alipay app/payment flow
- WeChat UX:
  - PC: show QR code in page
  - non-WeChat H5: prefer H5 pay; if unavailable, tell the user to open in WeChat
  - WeChat environment: prefer MP/JSAPI pay; if unavailable, fall back to H5 pay
- Payment success is confirmed by backend order state, webhook, and/or query, not only frontend return.

### OpenAI advanced scheduling

- OpenAI advanced scheduling is supported.
- It is disabled by default.
- Admin can enable it explicitly.

## Architecture

Keep `users` as the account owner table and move login identities, channel mappings, pending auth state, and first-bind grant idempotency into dedicated tables and services. Keep email login working while progressively introducing unified identity reads and writes.

Payment uses a similar split between user-visible display methods and backend provider sources. Frontend works only with stable display methods while backend resolves to the currently active source and capability matrix.

Compatibility is a first-class concern: migrations are additive, reads are compatibility-aware, and rollout must tolerate existing `main` data and short-lived frontend/backend version skew.

## Data Model

### `users`

Preserve existing account ownership and local-login fields. Extend or use:

- `email`
- `password_hash`
- `totp_enabled`
- `signup_source`
- `last_login_at`
- `last_active_at`

The `users` table remains the primary business subject for balance, concurrency, subscriptions, permissions, and profile.

### `auth_identities`

Represents all canonical login or bindable identities.

Fields:

- `user_id`
- `provider_type`: `email`, `linuxdo`, `oidc`, `wechat`
- `provider_key`
- `provider_subject`
- `verified_at`
- `issuer`
- `metadata`
- timestamps

Uniqueness:

- `provider_type + provider_key + provider_subject` must be unique

Rules:

- email identity uses canonicalized local email
- LinuxDo uses stable provider subject
- OIDC uses stable issuer + subject
- WeChat uses `unionid` as canonical subject

### `auth_identity_channels`

Stores channel-specific subject mappings for an identity.

Primary use:

- WeChat `open` / `mp` / payment channel mapping

Fields:

- `identity_id`
- `provider_type`
- `provider_key`
- `channel`
- `channel_app_id`
- `channel_subject`
- `metadata`
- timestamps

Rules:

- canonical WeChat identity still keys on `unionid`
- `openid` values live here as channel mappings

### `pending_auth_sessions`

Stores callback state between third-party callback and final account action.

Fields:

- `intent`
- `provider_type`
- `provider_key`
- `provider_subject`
- `target_user_id`
- `redirect_to`
- `resolved_email`
- `pending_password_hash`
- `upstream_identity_payload`
- `metadata`
- `email_verified_at`
- `password_verified_at`
- `totp_verified_at`
- `expires_at`
- `consumed_at`
- timestamps

Responsibilities:

- continue provider callback into register/login/bind flows
- persist nickname/avatar suggestions
- persist explicit adoption decisions
- survive navigation between auth pages

### `identity_adoption_decisions`

Persists user adoption preference for a specific identity.

Fields:

- `identity_id`
- `adopt_display_name`
- `adopt_avatar`
- `decided_at`
- timestamps

### `user_avatars`

Stores the currently effective custom avatar.

Fields:

- `user_id`
- `storage_provider`
- `storage_key`
- `url`
- `content_type`
- `byte_size`
- `sha256`
- timestamps

Rules:

- supports URL-backed and inline data-backed representations
- hard maximum payload size is `100KB`

### `user_provider_default_grants`

Stores idempotency state for source grants.

Fields:

- `user_id`
- `provider_type`
- `granted_at`
- timestamps

Responsibilities:

- prevent duplicate first-bind grants
- allow signup grants and first-bind grants to be reasoned about independently

## Identity Keys And Canonicalization

- Email canonical key: `lower(trim(email))`
- LinuxDo canonical key: provider subject from LinuxDo
- OIDC canonical key: `issuer + sub`
- WeChat canonical key: `unionid`

WeChat-specific rule:

- `openid` never becomes the primary stored identity key
- if only `openid` is available, login/bind fails with a configuration/identity error

## Core Flows

### Email register/login

- Existing email auth flow remains
- On email registration, create canonical `email` identity
- Apply `email` source signup defaults

### Third-party login with existing identity

- Resolve canonical identity
- Login mapped `user`
- Update `last_login_at`
- Do not issue signup or first-bind grants again

### Third-party first login with no identity

- Create `pending_auth_session`
- Frontend callback flow decides next action

Branches:

- no forced email binding:
  - user can create account directly
- forced email binding:
  - user must supply local email

If supplied local email already exists:

- tell the user the email already exists
- allow verify-and-bind-existing-account
- allow changing email to continue registration

On new account creation:

- create `users` row
- create canonical third-party identity
- apply source signup grants
- apply adoption choices if selected

### Bind third-party identity to current logged-in user

- current user starts bind flow
- callback resolves to `bind_current_user`
- bind canonical identity to current user
- if configured and first bind for that provider, apply first-bind grants
- present nickname/avatar replacement choice

### Bind existing account during first-login flow

- verify password for existing account
- if account requires TOTP, verify TOTP
- bind canonical identity to target account
- optionally apply first-bind grants
- present nickname/avatar replacement choice

### WeChat login and channel mapping

- environment chooses `mp` or `open`
- callback must resolve to `unionid`
- channel `openid` is optionally recorded in `auth_identity_channels`
- failure to obtain `unionid` aborts flow

### Avatar upload and delete

- URL avatar: validate and persist reference
- data URL avatar:
  - decode
  - validate image type
  - compress to `<=100KB`
  - persist database-backed inline representation
- delete removes current custom avatar entry

## Payment Routing Model

### User-visible methods

- `alipay`
- `wechat`

### Backend source abstraction

Each display method maps to exactly one active configured backend source:

- `official_alipay`
- `easypay_alipay`
- `official_wechat`
- `easypay_wechat`

Frontend submits display method only. Backend resolves display method to active source and capability set.

### Alipay routing

- PC: create QR-oriented result and show QR in page
- mobile: create jump/redirect-oriented result

### WeChat routing

- PC: QR result
- non-WeChat H5:
  - prefer H5 pay
  - if unavailable, show "open in WeChat" requirement
- WeChat environment:
  - prefer MP/JSAPI
  - if unavailable, fall back to H5 pay

### Payment completion

- frontend return restores context and UI state
- backend order state remains source of truth
- webhook and/or order query remain authoritative for fulfillment

## Admin Configuration Model

### Auth provider settings

- email registration and verification settings
- force email on third-party signup
- LinuxDo client settings
- OIDC issuer/client settings and provider display name
- WeChat `open` and `mp` settings with config-valid and health indicators

### Source default settings

Per source (`email`, `linuxdo`, `oidc`, `wechat`):

- default balance
- default concurrency
- default subscriptions
- grant on signup
- grant on first bind

### Payment settings

- active source for `alipay`
- active source for `wechat`
- source-specific credentials and enablement
- WeChat capability matrix:
  - QR available
  - H5 available
  - MP/JSAPI available

### Scheduling settings

- OpenAI advanced scheduling enabled/disabled
- default disabled

## Compatibility And Rollout

Compatibility is mandatory, especially for:

- existing email users
- existing LinuxDo users
- historical LinuxDo synthetic-email accounts

### Additive migrations

- preserve existing `users` data and behavior
- add identity and pending-session tables
- avoid destructive schema swaps

### Migration backfill

- backfill canonical `email` identities for valid existing email users
- backfill canonical `linuxdo` identities during migration for historical synthetic-email LinuxDo users
- backfill must be idempotent and repeatable

### Compatibility reads

During rollout:

- read new identity model first
- where necessary, retain compatibility logic for existing email and historical LinuxDo synthetic-email recognition

### Grant idempotency

- migration backfill must not trigger signup or first-bind grants
- first-bind grants must use explicit idempotency tracking

### API compatibility

Retain transitional support for legacy/new request and response shapes where needed, including:

- `pending_auth_token`
- `pending_oauth_token`
- old callback parsing expectations
- historical profile field mappings

### Settings and payment compatibility

- preserve existing payment configs and order semantics from `main`
- add new settings incrementally
- avoid rewriting the entire settings schema in one cutover

### Rolling upgrade tolerance

- do not assume simultaneous frontend/backend deployment
- new backend must tolerate short-lived older frontend request shapes

## Testing Strategy

### Repository tests

- identity upsert and lookup
- WeChat channel mapping
- pending auth session persistence
- source grant idempotency
- avatar persistence and delete
- migration backfill behavior

### Service tests

- direct login by existing identity
- first third-party signup
- forced email flow
- existing-email bind-existing-account flow
- first-bind grant on/off
- nickname/avatar adoption choices
- WeChat `unionid` required behavior
- payment routing resolution

### Handler and route tests

- LinuxDo/OIDC/WeChat callback handling
- bind-existing
- bind-current-user
- create-account
- TOTP continuation
- payment create and recovery

### Frontend tests

- third-party callback flow state machine
- register/login continuation
- profile bindings card
- avatar interactions
- payment page routing behavior
- admin settings UI

### Compatibility tests

- existing email users
- historical LinuxDo synthetic-email users
- historical payment config
- legacy auth payload field names
- historical payment result handling

## Implementation Phases

1. Add schema, migrations, compatibility backfill, and repository support
2. Implement unified identity services and pending auth session flows
3. Integrate profile binding, avatar, and adoption decision flows
4. Add per-source default grants and admin config surfaces
5. Rebuild payment routing abstraction and frontend payment UX
6. Add user-management sorting and OpenAI advanced scheduling switch
7. Run compatibility, rollout, and regression hardening

## External Constraints And Best Practices

Implementation must follow current primary-source guidance:

- OAuth 2.0 Security BCP (RFC 9700): strict redirect handling, state protection, mix-up resistant design
- PKCE (RFC 7636): use on authorization code flows where applicable
- OpenID Connect Core: stable issuer/subject handling for OIDC identities
- Account linking best practice: require explicit user confirmation or re-authentication before linking to existing accounts

References:

- RFC 9700: <https://www.rfc-editor.org/rfc/rfc9700>
- RFC 7636: <https://www.rfc-editor.org/rfc/rfc7636>
- OpenID Connect Core 1.0: <https://openid.net/specs/openid-connect-core-1_0.html>
- Auth0 account linking guidance: <https://auth0.com/docs/manage-users/user-accounts/user-account-linking>

## Audit Synthesis

The clean rebuild direction is not to copy either existing branch directly.

- `feat/auth-identity-foundation` has the better long-term model:
  - unified auth identities
  - pending auth sessions
  - identity adoption decisions
  - provider-scoped default grants
  - payment display-method abstraction
  - OpenAI advanced scheduler layering
- `personal-dev-branch` has the better real-world closure:
  - LinuxDo and WeChat callback flows are more operationally complete
  - profile binding and avatar UX is more complete
  - historical synthetic-email users are recognized and recovered in live flows
  - WeChat payment OAuth and recovery behavior is more complete
- Primary-source guidance supplies hard constraints for OAuth/OIDC, account linking, WeChat identity handling, and payment completion semantics.

The final rebuild must therefore:

- keep the `feat/auth-identity-foundation` data model direction
- absorb the strongest business-flow behavior from `personal-dev-branch`
- reject transitional or half-finished behavior from both branches
- treat compatibility and rollout as first-class implementation scope

## Keep / Adapt / Drop

### Keep

Keep these architectural choices essentially intact:

- `auth_identities`, `auth_identity_channels`, `pending_auth_sessions`, `identity_adoption_decisions`
- per-provider default grants with one-time grant tracking
- WeChat canonical identity plus channel mapping model
- pending-auth verification gates before final bind
- payment visible-method abstraction (`alipay`, `wechat`) decoupled from backend provider source
- OpenAI advanced scheduler layering and test-backed behavior

Keep these operational flow ideas from `personal-dev-branch`:

- LinuxDo pending identity callback flow
- WeChat pending identity callback flow
- profile bindings UX and “cannot disconnect last usable login method” rule
- separate WeChat login OAuth and WeChat payment OAuth entry points
- historical synthetic-email recognition logic as a migration bridge

### Adapt

These areas must be reimplemented with the same intent but stricter boundaries:

- third-party account creation from pending-auth state must be transactional and must not register a plain local user before identity finalization succeeds
- email identity lifecycle must become real dual-write state, not just one migration-time backfill
- `signup_source` must be backfilled more accurately for known historical third-party users
- WeChat payment recovery state must move from frontend-only storage to server-backed continuation state
- avatar adoption fetches must be security-hardened and failure-visible
- pending-auth payload modeling must clearly separate immutable upstream payload from mutable local metadata
- profile binding/avatar DTOs must be simplified to one authoritative backend contract instead of sprawling frontend fallback parsing
- admin settings should preserve capability while reducing duplicated or transitional config branches

### Drop

Drop these as long-term design choices:

- `user_external_identities` as the primary long-term identity model
- synthetic email as a long-term canonical identity representation
- OIDC as a side-path that does not participate in the same identity foundation as LinuxDo and WeChat
- frontend multi-endpoint probing and broad compatibility parsing once the clean branch becomes the sole supported contract
- unrelated branch noise such as generated-file churn, locale-only churn, or upstream merge residue as design inputs

## Audit-Driven Hard Constraints

The audit and source review establish these hard constraints:

### Auth

- all authorization-code providers use PKCE where applicable
- callback handling uses strict `redirect_uri` discipline and state validation
- OIDC identity key is `issuer + sub`
- existing-account linking after email conflict must require explicit user action plus local-account verification
- WeChat canonical identity key is `unionid`; `openid` is channel-scoped only

### Compatibility

- existing email users must continue to work with no manual intervention
- existing LinuxDo users must not split into duplicate accounts
- historical LinuxDo synthetic-email users must be backfilled into canonical LinuxDo identities during migration, not only lazily on next login
- migration backfills must not trigger signup or first-bind grants
- legacy `pending_auth_token` and `pending_oauth_token` contracts must remain accepted during rollout
- legacy auth/public setting aliases needed by older frontend builds must remain available during rollout
- existing payment configs and historical order semantics must remain valid

### Payment

- frontend return pages do not determine final payment success
- backend order state, webhook processing, and/or provider status query remain authoritative
- each visible method (`alipay`, `wechat`) may have only one active backend source at a time

## Known Risks To Eliminate In Implementation

These are specifically observed problems in the existing branches that the clean rebuild must eliminate:

- third-party forced-email account creation currently bypasses the provider-aware account creation path and can leave orphan local accounts if bind finalization fails
- post-migration email accounts are not fully dual-written into `auth_identities`
- avatar adoption currently risks silent failure and insecure outbound fetch behavior
- pending-auth payload responsibilities are internally inconsistent
- OIDC parity is incomplete in `personal-dev-branch`; it must become a first-class provider in the unified identity model
- WeChat union/open/channel identity handling is conceptually correct in the feature branch but still partially transitional across the codebase
- WeChat payment recovery in `personal-dev-branch` is frontend-local and not robust across tabs or concurrent attempts
- the existing pending-auth migration update is too destructive to reuse unchanged in a safer rollout
- historical provider provenance should not be permanently flattened to `signup_source = email`

## Rollout Gates

The rebuild is not ready for rollout until all of these are satisfied:

1. Identity schema and migration chain are linearized and production-safe.
2. Email identity backfill is complete and idempotent.
3. Historical LinuxDo synthetic-email backfill to canonical LinuxDo identity is complete and idempotent.
4. `signup_source` backfill is accurate for known historical provider-created users.
5. Dual token acceptance and required legacy field aliases are present.
6. Existing payment configs are normalized and verified against current frontend-visible capabilities.
7. New frontend flows are verified against mixed-version backend compatibility windows.
8. Duplicate-account creation, first-bind grants, and payment route selection have regression coverage.
