# Native Meta Muse provider integration

Tracking: [#7625](https://github.com/Wei-Shaw/sub2api/issues/7625).

This targets the consumer Muse app and its account-owned workspace. It implements
Sub2API's native account, gateway, runtime, billing, recovery, and operator surfaces.
Production wiring uses the native provider for standard VM single-message text
requests through the configured account proxy. Each account still requires live
verification before becoming schedulable. Public acceptance is recorded separately
from provider-port probes; see [protocol qualification](MUSE_NATIVE_PROTOCOL.md).
Synthetic conformance tests are not evidence of Meta compatibility.

## Implemented contracts

- `muse` platform and `session` account type. A bounded opaque `muse_session` document
  uses existing credential redaction and preserves secrets on ordinary account edits.
  It never selects the Meta developer API or OpenCode as a fallback.
- Every Muse workspace is assigned to one authenticated Sub2API user. A fresh
  verified principal/workspace identity binds local account aliases to one canonical
  PostgreSQL occupancy record. Separate side chats do not establish tenant isolation.
- Accounts start unschedulable with concurrency one. Verification must observe
  identity, explicitly allowed inference, and actual supported models/capabilities.
  Account edits invalidate the observed snapshot. Unknown models and usage are not
  populated from the third-party repos or plan names.
- Server-generated operations persist `submitting` before invoking a transport.
  Workspace leases survive client disconnects and are fenced across replicas.
  Expiry never authorizes another submission. Recovery probes the original remote
  task once and retains occupancy in `owner_review` when it cannot prove termination.
- Normalized event callbacks require the same operation/provider task, contiguous
  sequences, and identical duplicate events. Unrelated/proactive work cannot enter
  the response. Native adapters must finish callbacks before returning.
- Responses, Chat Completions, and Messages reuse Sub2API's wire serializers.
  The native path preserves client streaming and token limits, removes converter
  defaults intended for OpenAI, and rejects unknown or unsupported semantics.
  Tools, vision, reasoning, controls, and continuations require observed capability
  flags; compact, WebSocket, media, embeddings, and token counting are unsupported.
- Continuations require the original user/key, canonical workspace, generation,
  and a completed parent. The external provider ID is resolved by the server.
- Billing requires an explicit positive flat request price from the existing
  resolver, frozen with the user/group multiplier before submission. Simple mode
  uses explicit `test_free` pricing. Token pricing and request tiers are rejected.
  Only confirmed completed turns charge; failed/cancelled/rejected turns settle zero.
- Settlement uses the existing billing deduplication primitives and usage-log
  serializer in one SQL transaction, including balance/subscription, API-key quota
  and windows, native platform quota, usage-log link, and the turn settlement marker.
  The existing `(request_id, api_key_id)` unique usage index is reused.
  Cache invalidation follows commit; native platform quota preflight reads the DB
  directly, so a crash cannot lose its authoritative quota increment.
- A bounded worker retries local settlement with backoff, at most eight automatic
  attempts; operators can retry local settlement without replaying remote work.
  Remote completion is returned even when local settlement is temporarily pending.
  Another turn cannot be reserved in that workspace until settlement succeeds.
  Known session expiries support bounded renewal with backoff. Unknown expiries
  are never assigned a guessed refresh interval.
- Renewal locks the canonical workspace and validates the account snapshot before
  contacting the provider, preserves model mapping, invalidates verification, then
  re-verifies. Busy workspaces and stale callers do not invoke remote renewal.
- Admin routes use existing authentication/audit handling. Terminal owner resolution
  and local settlement retries use step-up middleware. The account editor shows
  connection status, observed allowance, and unresolved work. Releasing a workspace
  requires an explicit confirmed remote terminal outcome.

No response text, reasoning, request transcript, or session document is stored in
native turn metadata. It contains model attribution and provider-reported usage
only. `reported_usage = null` means unknown; zeros in legacy compatibility counters
are not measurements or a basis for token billing. Anthropic wire compatibility
may require zero-valued usage fields when the source supplies none.

## Native app authentication port

The MIT-licensed reference projects now share a concrete cookie-authentication path.
The native Go port accepts their `{cookies, expires}` / `cookie_expires` /
`cookies_exp` documents and CDP/Playwright HttpOnly cookie arrays. It imports
`hatch_sess`, `hatch_gw`, `hatch_vml`, and `hatch_native_auth_device`, using exact
Muse cookie-domain checks. `tools/muse-cookie-export` provides a Chromium exporter
that saves local JSON without sending credentials to a remote collector.
An authenticated consumer-app bootstrap on 2026-10-06 demonstrated that a fresh
login initially has the three persistent cookies; `GET /api/session` obtains
`hatch_vml`. Import therefore accepts a missing VM lease without sending an empty
cookie, while still requiring the three persistent credentials.
The same capture showed a host-only `hatch_vml` deletion followed by a valid
`.muse.ai` replacement in one response. Renewal evaluates all returned cookies
before classifying a session as expired. The native Go client successfully checked
the live session and obtained a VM lease from the three persistent credentials.

`POST /admin/muse/accounts/:id/authenticate` checks/renews via the fixed
`https://muse.ai/api/session` endpoint through the configured account proxy.
Redirects are disabled. Assigned-session metadata and returned `Set-Cookie` values
are validated; actual cookie expiry/deletion is honored. Unknown expiry stays
unknown. The port does not copy synthetic seven-day expiry extension, swallow auth
failures, clear shared browser cookies, or wake a VM during an authentication check.
HTTP 401 establishes a rejected session; HTTP 403 remains an unverified access
failure and does not tell an operator to replace otherwise valid cookies.

Credential rotation uses the existing canonical workspace lock and account/proxy
snapshot checks. It preserves opaque session extensions and model mapping. The
admin panel can check app cookies and renew sessions before inference is qualified.
A successful session check proves neither paid entitlement nor a canonical subject,
model catalog, chat-wire support, or live inference. The native provider performs those additional identity and catalog checks through
its verification port. An authentication-only check leaves the account unverified.

Authentication sources and copyright notices are recorded in
`THIRD_PARTY_NOTICES_MUSE.md`, pinned to the reviewed repository revisions. The
HTTP flow is verified with scripted transport fixtures and a live consumer-app
session on 2026-10-06. A fresh browser side-chat probe completed, and the native
provider now supports the restricted text scope described in protocol qualification. Native Go also completed the encrypted
handshake, read the actual model catalog, submitted fresh synthetic side chats,
and reconciled completed output/task state without resubmitting. Captures contain private session material
and are retained outside the repository; only sanitized contracts belong in tests.

## Remaining qualification gate

The native provider is based on the authorized consumer-app session and sanitized
captures. It does not derive endpoints from DOM output, Muse Code, Meta Model API,
or another product. The following evidence boundaries remain relevant, including
separate public gateway, billing, paid-account, and confidential-VM acceptance:

| Evidence | Required proof |
| --- | --- |
| Bootstrap/session | Actual cookie/token/CSRF fields, principal and workspace binding, session expiry and verified model access |
| Entitlement | The app plan and allowance actually used; missing values remain unknown |
| Turn submission | Request/acknowledgement correlation, definite rejection, durable acceptance and server task identity |
| Output | Complete and partial text, causal revisions, duplicate/out-of-order events, proactive side-chat filtering, terminal errors |
| Cancellation/recovery | Confirmed cancellation, lost acceptance acknowledgement, process loss, and reconciliation of the same task without replay |
| Renewal | Real renewal contract, stale credential rejection, concurrent alias/replica behavior and proxy/session affinity |
| App safeguards | Observed approval/Sentinel and any VM trust mode; never auto-approve actions or assume confidential VM is universal |
| Live acceptance | Current authorized Free account, correct public JSON/SSE, observable completion, one usage row and one charge after fault injection; paid access requires separate evidence |

Enabling an account in the UI cannot bypass this code-level qualification gate.
Successful local tests or CI do not establish live app compatibility.

## Verification

```sh
cd backend
go test -tags=unit ./...
go test -race -tags=unit ./internal/pkg/muse ./internal/service \
  -run 'TestMuse|TestCapabilities'
go test -race -tags=integration ./internal/pkg/muse
golangci-lint run --timeout=10m
cd ../frontend
pnpm run typecheck
pnpm run lint:check
pnpm run test:run
pnpm run build
```

The PostgreSQL tests create and remove only their own randomly named schemas.
They use a container by default or an explicitly configured disposable
`SUB2API_MUSE_TEST_DSN` PostgreSQL URL. Runtime tests use minimal admission fixtures;
billing tests use the generated ORM schema plus the authoritative billing, usage,
scheduler-outbox, and native migrations. They cover concurrent settlement, rollback
on usage-log failure, zero-charge failures, canonical locking, recovery/fencing,
credential edits, pricing mismatch, renewal, and owner resolution. They send no
Meta requests.

Application rollback preserves accepted-work and settlement records. Do not delete
native tables to release an uncertain remote task.
