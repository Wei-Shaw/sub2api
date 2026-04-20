# Auth Identity Payment Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild the auth identity, profile binding, payment routing, and OpenAI advanced scheduler foundation on top of a clean `origin/main` branch while preserving historical compatibility for existing email users, existing LinuxDo users, historical LinuxDo/WeChat/OIDC synthetic-email users, and historical WeChat `openid`-only records.
**Architecture:** A unified identity foundation centered on durable provider subjects (`email`, `linuxdo`, `oidc`, `wechat`) and transactional pending-auth sessions; backend-owned payment source routing behind stable frontend methods (`alipay`, `wxpay`); compatibility-first migration/backfill before feature enablement.
**Tech Stack:** Go, Gin, Ent, PostgreSQL, Redis, Vue 3, Pinia, TypeScript, Vitest, pnpm.

---

## Non-Negotiable Product Rules

- [ ] Preserve login continuity for existing email users, existing LinuxDo users, and historically migrated third-party users.
- [ ] During migration, backfill historical LinuxDo/WeChat/OIDC synthetic-email users into explicit third-party identities before first post-upgrade login whenever deterministic recovery is possible.
- [ ] During migration, surface historical WeChat `openid`-only records through explicit migration reports and remediation rules; do not silently reinterpret them as valid canonical identities.
- [ ] Keep existing email login and add third-party login/bind for `linuxdo`, `oidc`, and `wechat`.
- [ ] On first third-party login:
  - identity exists: direct login.
  - identity does not exist: start pending-auth flow.
  - local email binding is required only when system config says so.
  - upstream provider email verification never counts as local email verification.
- [ ] When user-entered and locally verified email already exists:
  - offer bind-existing-account after local re-authentication.
  - offer change-email-and-create-new-account.
  - when email binding is mandatory, do not allow bypass without changing to another email.
- [ ] On first third-party login or first third-party bind, provider nickname/avatar must be presented as independent replace options for the current nickname and avatar. They are not auto-applied.
- [ ] Source-specific initial grants must support per-source defaults for balance, concurrency, and subscriptions.
- [ ] Default grant timing: on successful new-account creation.
- [ ] Optional grant timing: on first successful bind for the configured source.
- [ ] Migration/backfill must never trigger first-bind or first-signup grants retroactively.
- [ ] Avatar profile supports:
  - direct URL storage.
  - image data URL upload compressed to `<=100KB` before storing in DB.
  - explicit delete.
- [ ] Admin user management must expose and sort by `last_login_at` and `last_active_at`.
- [ ] WeChat login rules:
  - WeChat environment uses MP login.
  - non-WeChat browser uses Open/QR login.
  - canonical identity uses `unionid`.
  - when `unionid` is unavailable, fail the login/bind flow under the approved option-1 policy.
- [ ] OIDC rules:
  - browser authorization-code flow always uses PKCE `S256`.
  - discovery issuer and ID token `iss` must match exactly.
  - `userinfo.sub` must match ID token `sub` when UserInfo is used.
  - upstream `email_verified` does not satisfy local email verification.
- [ ] Payment UI rules:
  - user-facing methods stay `支付宝` and `微信支付`.
  - backend decides whether each method routes to official provider instance or EasyPay.
  - at runtime, each visible method may only have one active source.
- [ ] Alipay rules:
  - PC: in-page QR.
  - mobile browser: jump to Alipay payment.
- [ ] WeChat Pay rules:
  - PC: in-page QR.
  - WeChat H5: MP/JSAPI first, fallback to H5 pay.
  - non-WeChat H5: H5 pay, or prompt to open in WeChat when unavailable.
- [ ] Payment success pages are informational only; actual fulfillment depends on webhook or server-side reconciliation.
- [ ] WeChat in-app payment requiring `openid` must use a dedicated server-backed payment OAuth resume flow rather than frontend-only recovery state.
- [ ] OpenAI advanced scheduler is available but default-disabled.

## Hard Technical Constraints From Audit

- [ ] Browser-based third-party auth must use Authorization Code + PKCE `S256`.
- [ ] PKCE must not be admin-configurable off for browser authorization-code providers.
- [ ] OIDC identity primary key must be `(issuer, subject)`, not email.
- [ ] Email equality must never auto-link accounts.
- [ ] Bind-existing-account must require explicit local re-authentication and TOTP verification when enabled.
- [ ] Bind-current-user must originate from an already-authenticated local user and preserve explicit bind intent across callback completion.
- [ ] OAuth redirect URI must be fixed server config, exact-match, and never derived from user input.
- [ ] User-supplied redirect may only choose a normalized same-origin internal route after completion.
- [ ] WeChat canonical identity must be `unionid`; `openid` remains channel/app-scoped support data only.
- [ ] Every canonical identity uniqueness rule must include provider namespace (`provider_key`) consistently.
- [ ] Callback completion must use backend session completion or a one-time opaque exchange code that is short-lived, one-time, browser-session-bound, `POST`-redeemed, and unusable as a bearer token.
- [ ] Every payment order must snapshot the selected provider instance plus the order-time verification inputs required for callback verification, reconciliation, refund, and audit.
- [ ] Frontend must not receive first-party bearer tokens through callback URL fragments in the rebuilt flow.
- [ ] Public payment result polling must not expose order data by raw `out_trade_no` alone; use authenticated lookup or signed opaque result token.
- [ ] WeChat Pay webhook handling must verify signature, decrypt payload, and compare `appid`, `mchid`, `out_trade_no`, `amount`, `currency`, and provider trade state against the order snapshot before fulfillment.

## Baseline Notes

- [ ] Current clean branch head when this plan was written: `721d7ab3`.
- [ ] Baseline backend verification on clean `origin/main`: `cd backend && go test ./...` passes.
- [ ] Baseline frontend verification on clean `origin/main`: `cd frontend && pnpm test:run` currently fails in unrelated existing suites. New work must add targeted tests and avoid claiming full frontend green until those baseline failures are addressed separately.
- [ ] Existing migration directory currently ends at `107_*`; this rebuild reserves `108` through `111`.

## Target File Map

### New backend migrations

- [ ] `backend/migrations/108_auth_identity_foundation_core.sql`
- [ ] `backend/migrations/109_auth_identity_compat_backfill.sql`
- [ ] `backend/migrations/110_pending_auth_and_provider_default_grants.sql`
- [ ] `backend/migrations/111_payment_routing_and_scheduler_flags.sql`

### New or rebuilt Ent schema

- [ ] `backend/ent/schema/auth_identity.go`
- [ ] `backend/ent/schema/auth_identity_channel.go`
- [ ] `backend/ent/schema/pending_auth_session.go`
- [ ] `backend/ent/schema/identity_adoption_decision.go`

### New or rebuilt backend repositories/services/handlers

- [ ] `backend/internal/repository/user_profile_identity_repo.go`
- [ ] `backend/internal/repository/user_profile_identity_repo_contract_test.go`
- [ ] `backend/internal/repository/auth_identity_migration_report.go`
- [ ] `backend/internal/service/auth_identity_flow.go`
- [ ] `backend/internal/service/auth_identity_flow_test.go`
- [ ] `backend/internal/service/auth_pending_identity_service.go`
- [ ] `backend/internal/service/auth_pending_identity_service_test.go`
- [ ] `backend/internal/service/payment_config_service.go`
- [ ] `backend/internal/service/payment_order.go`
- [ ] `backend/internal/service/payment_order_lifecycle.go`
- [ ] `backend/internal/service/payment_fulfillment.go`
- [ ] `backend/internal/service/payment_resume_service.go`
- [ ] `backend/internal/service/payment_resume_service_test.go`
- [ ] `backend/internal/service/openai_account_scheduler.go`
- [ ] `backend/internal/handler/auth_pending_identity_flow.go`
- [ ] `backend/internal/handler/auth_linuxdo_oauth.go`
- [ ] `backend/internal/handler/auth_oidc_oauth.go`
- [ ] `backend/internal/handler/auth_wechat_oauth.go`
- [ ] `backend/internal/handler/auth_handler.go`
- [ ] `backend/internal/handler/user_handler.go`
- [ ] `backend/internal/handler/payment_handler.go`
- [ ] `backend/internal/handler/payment_webhook_handler.go`
- [ ] `backend/internal/handler/admin/user_handler.go`
- [ ] `backend/internal/handler/admin/setting_handler.go`

### New or rebuilt frontend API/store/views/components

- [ ] `frontend/src/api/auth.ts`
- [ ] `frontend/src/api/user.ts`
- [ ] `frontend/src/api/payment.ts`
- [ ] `frontend/src/api/admin/settings.ts`
- [ ] `frontend/src/api/admin/users.ts`
- [ ] `frontend/src/stores/auth.ts`
- [ ] `frontend/src/stores/payment.ts`
- [ ] `frontend/src/components/auth/ThirdPartyAuthCallbackFlow.vue`
- [ ] `frontend/src/components/auth/LinuxDoOAuthSection.vue`
- [ ] `frontend/src/components/auth/OidcOAuthSection.vue`
- [ ] `frontend/src/components/auth/WechatOAuthSection.vue`
- [ ] `frontend/src/components/user/profile/ProfileAccountBindingsCard.vue`
- [ ] `frontend/src/components/user/profile/ProfileInfoCard.vue`
- [ ] `frontend/src/views/auth/LinuxDoCallbackView.vue`
- [ ] `frontend/src/views/auth/OidcCallbackView.vue`
- [ ] `frontend/src/views/auth/WechatCallbackView.vue`
- [ ] `frontend/src/views/user/ProfileView.vue`
- [ ] `frontend/src/views/user/PaymentView.vue`
- [ ] `frontend/src/views/user/PaymentQRCodeView.vue`
- [ ] `frontend/src/views/user/PaymentResultView.vue`

## Phase 1: Migration And Compatibility Foundation

### Task 1. Create core identity schema migration

- [ ] Implement `backend/migrations/108_auth_identity_foundation_core.sql` with:
  - `auth_identities`
  - `auth_identity_channels`
  - `pending_auth_sessions`
  - `identity_adoption_decisions`
  - `users.last_login_at`
  - `users.last_active_at`
  - grant-tracking columns/tables required to prevent double-award
- [ ] Add uniqueness/index rules:
  - one canonical identity per `(provider, provider_key, provider_subject)`
  - one channel record per `(provider, provider_channel, provider_app_id, provider_channel_subject)`
  - one adoption decision per pending session
- [ ] Model `pending_auth_sessions` so immutable upstream claims and mutable local flow state are stored separately; do not reintroduce a mixed `metadata` catch-all.
- [ ] Preserve null-safe compatibility defaults so historical rows remain readable before backfill finishes.
- [ ] Add explicit rollback blocks only where safe; never repeat the destructive pattern observed in old `112_update_pending_auth_sessions.sql`.

### Task 2. Materialize historical identities before runtime

- [ ] Implement `backend/migrations/109_auth_identity_compat_backfill.sql` to backfill:
  - existing email users into `auth_identities(provider=email, provider_subject=normalized_email)`
  - historical LinuxDo users into `auth_identities(provider=linuxdo, provider_subject=linuxdo_subject)`
  - historical synthetic-email LinuxDo users into explicit LinuxDo identity rows by parsing legacy email mode and legacy provider metadata
  - historical synthetic-email WeChat users into explicit WeChat identities where `unionid` or equivalent deterministic provider identity is recoverable
  - historical synthetic-email OIDC users into explicit OIDC identities where deterministic provider identity is recoverable
  - profile/channel rows from historical `user_external_identities`-style data when present in upgraded databases
- [ ] Write migration report output in `backend/internal/repository/auth_identity_migration_report.go` so production can inspect unmatched rows, `openid`-only WeChat rows, and non-deterministic synthetic-email rows instead of silently skipping them.
- [ ] Set `signup_source` and provider provenance when recoverable from historical data. Do not flatten everything to `email`.

### Task 3. Provider default grant and scheduler config migration

- [ ] Implement `backend/migrations/110_pending_auth_and_provider_default_grants.sql` for:
  - provider-specific initial balance/concurrency/subscription defaults
  - grant timing flags: `on_signup`, optional `on_first_bind`
  - email-required-on-third-party-signup flags
  - profile avatar storage columns/settings
- [ ] Implement `backend/migrations/111_payment_routing_and_scheduler_flags.sql` for:
  - stable payment method to provider-instance routing
  - visible-method normalization from historical `supported_types`, `payment_mode`, and legacy aliases such as `wxpay_direct`
  - admin exclusivity flags for `alipay` and `wxpay`
  - advanced scheduler enable flag defaulting to disabled

### Task 4. Generate Ent and compile migration-safe model layer

- [ ] Add the schema definitions in:
  - `backend/ent/schema/auth_identity.go`
  - `backend/ent/schema/auth_identity_channel.go`
  - `backend/ent/schema/pending_auth_session.go`
  - `backend/ent/schema/identity_adoption_decision.go`
- [ ] Run:
  ```bash
  cd backend
  go generate ./ent
  ```
- [ ] Compile after generation:
  ```bash
  cd backend
  go test ./... -run '^$'
  ```
- [ ] Commit checkpoint:
  ```bash
  git add backend/migrations backend/ent/schema backend/ent
  git commit -m "feat: add auth identity foundation schema"
  ```

## Phase 2: Backend Identity Flow Rebuild

### Task 5. Build a single repository contract for identity lookups and grants

- [ ] Implement `backend/internal/repository/user_profile_identity_repo.go` with transactional helpers for:
  - get user by canonical identity
  - get user by channel identity
  - create canonical + channel identity together
  - bind identity to existing user after verified re-auth
  - record one-time provider grant award
  - record adoption preference decisions
  - update `last_login_at` and `last_active_at`
- [ ] Add repository contract coverage in `backend/internal/repository/user_profile_identity_repo_contract_test.go`.
- [ ] Enforce dual-write for email registration/login so `users.email` and `auth_identities(provider=email, ...)` stay consistent from this phase onward.
- [ ] Add repository coverage proving `last_login_at` and `last_active_at` use the required field names and are not silently replaced by derived `last_used_at` logic.

### Task 6. Rebuild transactional pending-auth service

- [ ] Implement `backend/internal/service/auth_pending_identity_service.go` and tests to own these flows:
  - create pending session from third-party callback
  - verify local email code
  - create new account from pending session with correct `signup_source`
  - bind pending identity to existing account after password/TOTP re-auth
  - apply configured provider defaults on the correct trigger only once
  - store provider nickname/avatar candidates and user opt-in replacement decisions independently
- [ ] Implement callback completion so pending auth can finish through backend session completion or a one-time exchange code:
  - short TTL
  - one-time use
  - browser-session binding
  - `POST` redemption only
  - safe mixed-version bridge to legacy pending-token aliases during rollout
- [ ] Keep pending session payload normalized:
  - provider identity fields live in typed columns/JSON structure
  - mutable local progression lives separately from immutable upstream claims
  - avoid the old branch’s mixed `metadata` and `upstream_identity_payload` ambiguity
- [ ] Do not call plain email registration helpers from this flow. The old feature branch bug where pending third-party signup fell back to `RegisterWithVerification` must not reappear.

### Task 7. Rebuild provider callback adapters

- [ ] Refactor these handlers to thin adapters over the shared pending-auth service:
  - `backend/internal/handler/auth_linuxdo_oauth.go`
  - `backend/internal/handler/auth_oidc_oauth.go`
  - `backend/internal/handler/auth_wechat_oauth.go`
- [ ] For OIDC:
  - require PKCE `S256`, `state`, and `nonce`
  - validate discovery issuer, `iss`, `aud`, optional `azp`, `exp`, and `nonce`
  - verify `userinfo.sub == id_token.sub` when UserInfo is used
  - persist canonical identity as `(issuer, sub)`
- [ ] For WeChat:
  - MP flow in WeChat UA
  - Open/QR flow outside WeChat UA
  - website login uses authorization-code flow and persists channel/app binding
  - persist channel identity by `(channel, appid, openid)`
  - persist canonical identity by `unionid`
  - hard-fail when `unionid` is absent under the approved product policy
- [ ] Replace callback URL fragment token delivery with backend session completion or one-time exchange code consumed by `frontend/src/stores/auth.ts`.

### Task 8. Rebuild auth endpoints and profile binding endpoints

- [ ] Implement `backend/internal/handler/auth_pending_identity_flow.go` for:
  - fetch pending session summary
  - submit verified email
  - choose create-new-account or bind-existing-account
  - submit nickname/avatar replacement choices
- [ ] Make bind-existing-account and bind-current-user flows explicit:
  - no automatic linking on matching email
  - fresh password/TOTP proof is scoped to the intended target account only
  - no automatic metadata merge beyond explicitly selected nickname/avatar adoption
- [ ] Update `backend/internal/handler/auth_handler.go` and `backend/internal/handler/user_handler.go` to expose:
  - current bindings summary
  - start-bind endpoints for LinuxDo/OIDC/WeChat
  - disconnect endpoints with safety checks
  - avatar upload/delete endpoints
- [ ] Avatar handling requirements:
  - allow external URL
  - allow data URL upload
  - compress image payload to `<=100KB`
  - store compressed value in DB
  - deleting custom avatar must not implicitly resurrect stale provider avatar unless the user explicitly chooses provider avatar again

### Task 9. Add admin visibility and sorting

- [ ] Update `backend/internal/handler/admin/user_handler.go` and supporting query/service code so admin list supports:
  - `last_login_at`
  - `last_active_at`
  - sorting by both
  - binding/provider summary columns
- [ ] Update `backend/internal/handler/admin/setting_handler.go` and setting service code for:
  - provider initial grant config
  - mandatory-email-on-third-party-signup config
  - payment source exclusivity config
  - advanced scheduler toggle

### Task 10. Backend verification checkpoint

- [ ] Run targeted backend tests:
  ```bash
  cd backend
  go test ./internal/repository -run 'TestUserProfileIdentity|TestAuthIdentityMigration'
  go test ./internal/service -run 'TestAuthIdentityFlow|TestPendingAuthIdentity|TestOpenAIAccountScheduler'
  go test ./internal/handler -run 'TestLinuxDo|TestOidc|TestWechat|TestPaymentWebhook'
  go test ./...
  ```
- [ ] Commit checkpoint:
  ```bash
  git add backend
  git commit -m "feat: rebuild auth identity backend flows"
  ```

## Phase 3: Frontend Third-Party Flow And Profile UX

### Task 11. Rebuild callback flow UI around pending session decisions

- [ ] Rebuild `frontend/src/components/auth/ThirdPartyAuthCallbackFlow.vue` so it:
  - loads pending-session summary from backend
  - shows provider nickname/avatar candidates
  - lets user independently choose nickname replacement and avatar replacement
  - handles create-new-account vs bind-existing-account
  - enforces verified local email before completion when required
  - handles “email already exists” by branching to bind-existing-account or change-email-and-create-new-account
- [ ] Update:
  - `frontend/src/views/auth/LinuxDoCallbackView.vue`
  - `frontend/src/views/auth/OidcCallbackView.vue`
  - `frontend/src/views/auth/WechatCallbackView.vue`
  - `frontend/src/api/auth.ts`
  - `frontend/src/stores/auth.ts`
- [ ] Replace any token-fragment bootstrap with backend session completion or one-time exchange code flow.
- [ ] During rollout, keep temporary compatibility readers for legacy pending-token aliases behind a bounded bridge contract and explicit removal step.

### Task 12. Rebuild profile account binding and avatar UX

- [ ] Rebuild `frontend/src/components/user/profile/ProfileAccountBindingsCard.vue` to:
  - show linked LinuxDo/OIDC/WeChat providers
  - start bind/unbind flows
  - show provider avatars and nicknames as reference only
  - prevent unsafe disconnect when it would strand the account
- [ ] Rebuild `frontend/src/components/user/profile/ProfileInfoCard.vue` and `frontend/src/views/user/ProfileView.vue` to:
  - support avatar URL entry
  - support data URL upload/compression preview
  - support avatar delete
  - clearly separate current profile nickname/avatar from provider-sourced suggested nickname/avatar

### Task 13. Add frontend tests for rebuilt auth/profile flows

- [ ] Add or update:
  - `frontend/src/components/auth/__tests__/ThirdPartyAuthCallbackFlow.spec.ts`
  - `frontend/src/components/auth/__tests__/LinuxDoCallbackView.spec.ts`
  - `frontend/src/components/auth/__tests__/WechatCallbackView.spec.ts`
  - `frontend/src/components/user/profile/__tests__/ProfileAccountBindingsCard.spec.ts`
  - `frontend/src/components/user/profile/__tests__/ProfileInfoCard.spec.ts`
- [ ] Cover:
  - email-required branch
  - email-conflict branch
  - bind-existing-account with re-auth prompt
  - nickname replacement only
  - avatar replacement only
  - neither replacement
  - avatar delete after prior provider adoption

## Phase 4: Payment Routing Rebuild

### Task 14. Normalize payment routing backend

- [ ] Rebuild `backend/internal/service/payment_config_service.go` to expose a stable method-routing contract:
  - frontend visible methods remain `alipay` and `wxpay`
  - admin chooses which provider instance serves each method
  - runtime validation guarantees only one active source per visible method
- [ ] Add migration logic and tests to normalize historical provider-instance config:
  - `supported_types`
  - `payment_mode`
  - legacy aliases such as `wxpay_direct`
  - historical limit config
- [ ] Rebuild `backend/internal/service/payment_order.go` and `backend/internal/service/payment_order_lifecycle.go` so order creation snapshots:
  - visible method
  - selected provider instance id
  - provider type
  - provider capability mode
  - verification-critical provider fields needed for later callback/query/refund validation
- [ ] Rebuild `backend/internal/handler/payment_handler.go` for UX rules:
  - Alipay PC: QR page
  - Alipay mobile: direct jump
  - WeChat PC: QR page
  - WeChat H5 in WeChat: MP/JSAPI first, fallback to H5
  - WeChat H5 outside WeChat: H5 or “open in WeChat” prompt when unavailable
- [ ] Never derive canonical return URL from `Referer`; use configured or signed internal callback targets only.
- [ ] Implement `backend/internal/service/payment_resume_service.go` so WeChat in-app payment OAuth resume is server-backed rather than localStorage-backed:
  - create `oauth_required` resume context
  - persist amount/order_type/plan_id/visible method/redirect/state
  - redeem callback into same-origin internal resume target
  - expire and consume resume context safely

### Task 15. Make fulfillment and reconciliation provider-instance-safe

- [ ] Rebuild `backend/internal/handler/payment_webhook_handler.go` and `backend/internal/service/payment_fulfillment.go` so:
  - verification uses the order’s original provider instance
  - webhook processing is idempotent by provider event id and internal order id
  - missed webhook recovery uses server-side provider query, not frontend success return
- [ ] For WeChat Pay specifically, enforce:
  - fixed HTTPS `notify_url` with no query params
  - no dependency on user login state
  - signature verification before decrypt
  - APIv3 decrypt before business parsing
  - comparison of `appid`, `mchid`, `out_trade_no`, `amount`, `currency`, and trade state against the order snapshot
- [ ] Harden `frontend/src/views/user/PaymentResultView.vue` and `frontend/src/api/payment.ts` so result polling uses an authenticated order lookup or signed opaque token, not a raw public `out_trade_no` query.

### Task 16. Rebuild payment frontend views

- [ ] Rebuild `frontend/src/views/user/PaymentView.vue`, `frontend/src/views/user/PaymentQRCodeView.vue`, and `frontend/src/stores/payment.ts` so:
  - only two buttons are shown to user: `支付宝` and `微信支付`
  - frontend does not leak official-vs-EasyPay distinction
  - route-specific copy handles QR, jump, MP, H5 fallback correctly
- [ ] Rebuild WeChat in-app payment resume UX around the server-backed resume context:
  - handle `oauth_required`
  - continue from same-origin resume target
  - avoid long-lived localStorage as the source of truth
- [ ] Add or update:
  - `frontend/src/views/user/__tests__/PaymentView.spec.ts`
  - `frontend/src/views/user/__tests__/PaymentResultView.spec.ts`
  - backend webhook/payment routing tests

### Task 17. Payment verification checkpoint

- [ ] Run:
  ```bash
  cd backend
  go test ./internal/service -run 'TestPayment'
  go test ./internal/handler -run 'TestPayment'
  cd ../frontend
  pnpm test:run src/views/user/__tests__/PaymentView.spec.ts src/views/user/__tests__/PaymentResultView.spec.ts
  ```
- [ ] Commit checkpoint:
  ```bash
  git add backend frontend
  git commit -m "feat: rebuild payment routing foundation"
  ```

## Phase 5: Scheduler, Rollout, And Final Compatibility Pass

### Task 18. Gate advanced scheduler behind explicit config

- [ ] Update `backend/internal/service/openai_account_scheduler.go` and related admin setting surfaces so:
  - advanced scheduler remains compiled and testable
  - default runtime state is disabled
  - enablement is explicit through admin settings
  - legacy scheduling behavior remains default on upgrade
- [ ] Add targeted coverage in `backend/internal/service/openai_account_scheduler_test.go`.

### Task 19. Complete compatibility and rollout safety checks

- [ ] Add migration/repository tests covering:
  - historical email-only user login after upgrade
  - historical LinuxDo user login after upgrade
  - historical synthetic-email LinuxDo user login after upgrade
  - historical synthetic-email WeChat user login after upgrade
  - historical synthetic-email OIDC user login after upgrade
  - historical WeChat `openid`-only rows are reported or explicitly remediated
  - no retroactive grant replay during migration
  - first-bind grant fires once only when enabled
  - email identity dual-write stays consistent
  - bind-existing-account requires password and TOTP where configured
  - mixed-version callback token bridge works during rollout and is removable afterward
  - historical payment config is normalized into visible-method routing without refund/query regression
- [ ] Add deploy sequencing note to release docs or internal runbook:
  1. deploy schema and backfill release.
  2. inspect migration report for unmatched rows.
  3. deploy backend identity/payment compatibility code with exchange bridge and legacy token aliases still enabled.
  4. deploy frontend callback/profile/payment UI using session completion, exchange code, and server-backed WeChat payment resume.
  5. remove legacy callback/token parsing after mixed-version window closes.
  6. enable strict email-required signup or provider bind grants only after metrics are healthy.

### Task 20. Final verification and handoff

- [ ] Run final backend verification:
  ```bash
  cd backend
  go test ./...
  ```
- [ ] Run targeted frontend verification:
  ```bash
  cd frontend
  pnpm test:run \
    src/components/auth/__tests__/ThirdPartyAuthCallbackFlow.spec.ts \
    src/components/auth/__tests__/LinuxDoCallbackView.spec.ts \
    src/components/auth/__tests__/WechatCallbackView.spec.ts \
    src/components/user/profile/__tests__/ProfileAccountBindingsCard.spec.ts \
    src/components/user/profile/__tests__/ProfileInfoCard.spec.ts \
    src/views/user/__tests__/PaymentView.spec.ts \
    src/views/user/__tests__/PaymentResultView.spec.ts
  ```
- [ ] Run focused manual smoke checks:
  - email login with existing account
  - LinuxDo existing-account login after migration
  - WeChat synthetic-email account login after migration
  - OIDC synthetic-email account login after migration
  - third-party first login create-new-account path
  - third-party first login bind-existing-account path
  - first third-party bind with optional nickname/avatar replacement
  - PC Alipay QR
  - mobile Alipay jump
  - PC WeChat QR
  - WeChat H5 MP/JSAPI path
  - WeChat in-app OAuth resume path
  - non-WeChat H5 fallback path
- [ ] Commit final checkpoint:
  ```bash
  git add docs backend frontend
  git commit -m "feat: rebuild auth identity and payment foundation"
  ```

## Review Checklist

- [ ] No flow still relies on provider email equality for account linking.
- [ ] No flow still creates third-party users through plain email registration helpers.
- [ ] No callback still returns first-party bearer tokens in URL fragments.
- [ ] No callback completion path can be replayed as a bearer token substitute.
- [ ] No payment result view trusts provider return page as authoritative fulfillment.
- [ ] No webhook verification path selects provider credentials from “currently active config” instead of the order snapshot.
- [ ] Existing email users, historical LinuxDo/WeChat/OIDC users, and `openid`-only WeChat remediation cases are covered by migration tests.
- [ ] Avatar adoption and deletion semantics are explicit and reversible.
- [ ] Grant timing is source-aware and one-time only.
