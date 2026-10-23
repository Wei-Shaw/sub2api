# Devin subscription accounts

Devin uses a **Devin CLI session token**, not a `devin_...` REST API key. The
adapter is based on oh-my-pi commit
`4049c62fbb0a7f5068954d01f11a73112c942a11`; see [license notices](../THIRD_PARTY_NOTICES.md).
It communicates with the Codeium/Devin Connect protobuf service, not an
OpenAI-compatible upstream endpoint.

## Setup

1. Sign in to your own Devin account with the official Devin CLI or oh-my-pi's
   Devin login. Your account/team must have CLI and model access.
2. Obtain the **CLI session token** from that authorized login. In oh-my-pi's
   Devin OAuth credential, this is the `access` value (the token exchange response
   calls it `token`). Both the bare token and `devin-session-token$`-prefixed form
   are accepted. Do not paste a whole credential database, browser cookie, REST
   API key, or another user's token.
3. In **Admin → Accounts → Add account → Devin**, paste it into **Devin CLI session
   token**. Assign the account to a Devin group; create one under Groups first if
   needed. Existing composite groups can route to the Devin platform too.
4. Use the account's model synchronization action to discover models available to
   **that credential**. The fallback IDs are `swe-1-6` and `swe-1-6-fast`; they are
   not a promise that every account has access. Model mapping/whitelists work as
   for other platforms. Configure group/channel pricing for the models you serve;
   this integration does not invent subscription prices or remaining quota.
5. Create a sub2api API key bound to the group. Use that key with `/v1/chat/completions`,
   `/v1/responses`, or `/v1/messages`.

This version imports an existing CLI token; it does **not** run a browser login or
read local CLI/browser credential stores automatically. Treat the token as a
password. Account API responses redact it; leaving the replacement field blank
when editing retains it.

### Expiration and renewal

The upstream login uses browser PKCE at `https://app.devin.ai/auth/cli/continue`
and exchanges `code` + `code_verifier` as JSON at
`https://api.devin.ai/auth/cli/token`. There is **no refresh grant** in the inspected
upstream implementation. Although oh-my-pi duplicates the token in its `refresh`
field, that field cannot renew it.

When a token expires or is revoked, sign in again and replace the CLI token in
**Edit account**, then re-enable/reset the account if it was marked unhealthy.
JWT expiration is used only as a local expiry hint, not as proof of authentication.
Opaque tokens are validated by the upstream. Automatic token refresh is unsupported.

## Client example

```bash
curl "$SUB2API_URL/v1/chat/completions" \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H 'Content-Type: application/json' \
  -H 'session_id: my-conversation-1' \
  -d '{"model":"swe-1-6","stream":true,"messages":[{"role":"user","content":"Hello"}]}'
```

Use a unique stable `session_id` per conversation and replay the complete history
on subsequent turns. Session identifiers are namespaced by sub2api API key and
selected account before being sent to Devin, including router assignment calls.
Do not depend on `previous_response_id` or server-stored conversation replay.

## Protocol and operational limits

- The adapter converts text, function tools/results, visible reasoning and token
  usage into the existing Chat Completions pipeline. Existing converters provide
  Responses and Anthropic Messages output in streaming and non-streaming modes.
  Reasoning is exposed as plain text; opaque upstream signatures and native
  assistant-message identities are not round-tripped through client history.
- Upstream model discovery is account-scoped. Router models must be assigned to a
  concrete model and assignment JWT; they are never sent as literal chat UIDs.
- The known image-blind IDs `swe-1-6` and `swe-1-6-fast` are text-only, matching
  the inspected upstream implementation. Other models use their discovered image
  capability; images must be inline base64, and remote fetching is not provided.
  Image behavior was not part of the authenticated live smoke test.
- Hosted tools (web search, code execution), audio/video generation, structured
  output formats, encrypted/foreign signed reasoning replay, multiple choices,
  WebSocket ingress, remote compaction and server-stored conversation continuation
  are unsupported. Send ordinary function tool schemas and complete paired
  call/result history instead. Explicit `reasoning_effort`, Anthropic `thinking`
  or `output_config` controls are unsupported: choose the desired model variant
  from the account's catalog. Cache policy is provider-controlled; Anthropic
  cache-control hints do not override Devin's ephemeral system cache policy.
- Subscription balance/remaining quota is not inferred from token usage. Local
  usage/billing records use the upstream's token counters, including cache reads
  and writes. Model-specific access and rate limits remain upstream-controlled.
- HTTP errors and Connect error trailers are failures, including trailers inside
  HTTP 200 responses. Truncated streams must not be interpreted as success.
  Frames are limited to 16 MiB (compressed and expanded), completions to 32 MiB,
  each tool's arguments to 4 MiB, and each completion to 1,024 tool calls.
  Protobuf messages are additionally limited to 65,536 fields per nesting level
  to prevent tiny scalar fields from causing excessive decoded-memory allocation.
- All upstream traffic uses the account's configured proxy and the shared HTTP
  transport. Authentication starts at `https://server.codeium.com`. An upstream
  regional server URL must be an HTTPS origin under `codeium.com`, `windsurf.com`
  or `devin.ai`, and must also pass the operator's outbound URL policy. If URL
  allowlisting is enabled, allow the needed provider hosts. Arbitrary custom
  endpoints and private origins are not accepted for these subscription tokens.
  HTTP redirects are disabled: 307/308 redirects must not replay credential-bearing
  protobuf bodies to another host. Provider hosts must resolve to public IPs even
  when the global URL allowlist is disabled.

## Fusion support status

**Native Fusion is not implemented or live-validated by this adapter.** Fusion
pairs a lead model with a sidekick under a session-level orchestrator; it is not
interchangeable with the `adaptive` router's `AssignModel` handshake. The proxy
excludes `fusion` / `fusion-*` entries from its supported catalog and rejects
explicit Fusion selections before sending an inference request, rather than
silently running only the lead or incorrectly treating a pairing as Adaptive.

The [official Fusion documentation](https://docs.devin.ai/cli/fusion) requires a
supported paid plan and Devin CLI 3000.10.20+ (or Desktop 3.10.0+); free/trial and
legacy credit-based plans are excluded. A live read-only diagnostic of the
validation account found 587 Fusion-related raw catalog entries, all disabled,
so no Fusion inference request was attempted. This is an access limitation, not
a passed Fusion test.

Current [oh-my-pi discovery code](https://github.com/can1357/oh-my-pi/blob/5d72f5d3f49a802e51d04c810002eb1830440172/packages/catalog/src/discovery/devin.ts)
can translate a pairing ID into its standalone lead UID, but explicitly does not
run the sidekick. That behavior must not be represented as full Fusion support.
A Fusion-enabled account and verification of the native lead/sidekick session
contract are still needed before adding genuine Fusion support here.

Read-only capability check (no inference usage):

```bash
cd backend
DEVIN_LIVE_TEST=1 DEVIN_SESSION_TOKEN_FILE=/absolute/path/to/private-token \
  go test -tags=live ./internal/pkg/devin \
  -run '^TestDevinLiveFusionAvailability$' -v -count=1
```

## Verification

Use the repository's Go 1.27.2 toolchain and pnpm 9.x:

```bash
cd backend
go test ./internal/pkg/devin
go test -tags=unit ./internal/service ./internal/handler ./internal/handler/admin -run Devin
# Broader regression gate:
go test -tags=unit ./...
cd ../frontend
pnpm run lint:check
pnpm run typecheck
pnpm run test:run
pnpm run build
```

The automated tests use mocked upstream protobuf/Connect responses; they do not
prove current subscription entitlement or live service compatibility. A real
smoke test requires the administrator's own authorized token and may consume
subscription usage. No live credentials or paid calls are needed for the fixture
suite.

### Implementation audit

| Requirement | Implementation and regression evidence |
| --- | --- |
| Configure/edit a subscription account without exposing secrets | `service/devin.go`, admin account wiring, `admin_account_devin_test.go`, frontend `DevinAccounts.spec.ts`; connection-test success/failure paths assert SSE response headers |
| Group assignment, platform registration and model selection | domain platform catalog, group binding tests, `gateway_handler_devin_test.go`, `upstream_models_devin_test.go` |
| All three client protocols, streaming/non-streaming, function calls | `devin_gateway_test.go` exercises the real conversion paths with mocked Connect responses |
| Auth, router assignment, model discovery and exact wire mapping | `internal/pkg/devin/devin_test.go` uses pinned protobuf-field fixtures |
| Errors, cancellation, truncated/compressed streams and resource limits | package stream/error/cancellation tests, `limits_test.go`, gateway failure tests; race tests pass |
| Usage and scheduler integration, session/proxy isolation | gateway token/cache counter assertions, `devin_scheduling_test.go`, proxy/session tests; other-provider endpoint and Claude quota fallthrough are blocked |
| Unsupported inputs are not silently discarded | `devin_validation_test.go`, package input rejection tests; tool errors and plain-text reasoning history are preserved |
| License retention | root `THIRD_PARTY_NOTICES.md`, runtime `resources/NOTICE-oh-my-pi.txt`, release archive notice entry |

Verified locally: backend build; focused backend/race tests; package `go vet`;
frontend lint, typecheck, build and all 2,880 tests. The full backend unit run
completed with one failure, `TestInflightEstimate_AccountMappingNoDBAndBoundedMemory`;
the same failure was reproduced in a clean worktree of unchanged HEAD. That
unrelated allocation assertion was not weakened or modified.

### Extended test results

- Native provider adapter: **90.1% statement coverage**, five shuffled race-test
  repetitions, real in-process HTTP chunked/gzip/cancellation tests, and
  **600,016 fuzz executions** across unary protobuf, Connect framing and request
  mapping. No fuzz failures were found in those bounded runs.
- Gateway/admin/repository Devin regressions: three race-test repetitions pass,
  including 307/308 credential-body redirect protection.
- Frontend: lint, typecheck, all **2,880 tests**, and production build pass using
  Node **20.20.2** and pnpm **9.15.9** (matching the CI major versions).
- Backend: embedded build and `go vet` checks pass. The full unit run still has
  the pre-existing allocation-test failure above; running the full suite with
  **only that named test excluded** passes. This exclusion is diagnostic, not a
  change to the normal test gate.
- Repository integration tests compile. The Docker-based suite was not executed
  because no Docker daemon is available. A disposable localhost instance was
  subsequently started successfully with PostgreSQL 16 and Redis, and admin login
  worked. Creating the test group was blocked with `ADMIN_COMPLIANCE_ACK_REQUIRED`
  (HTTP 423). The deployment commitment was **not accepted automatically**; the
  temporary services were stopped. Public gateway inference and database-backed
  billing checks therefore remain unverified in that live instance.
- Real Devin authentication endpoint: a deliberately invalid synthetic token
  was rejected with **HTTP 401**.
- A subsequent browser PKCE login and token exchange succeeded. Authenticated
  discovery returned `swe-1-6-slow` for the test account (not the static seed IDs).
  Using that discovered model, live **non-streaming text, streaming text/usage/
  termination, a function call, and a linked tool-result continuation all passed**.
  These are native-adapter checks, not a claim that the blocked public gateway
  checks above ran. No session token or authorization code was logged in the repo.

### Opt-in authenticated live tests

The live tests never read browser/CLI credential stores or reuse unrelated API
keys. Supply your own authorized CLI token through `DEVIN_SESSION_TOKEN`, or put
it in an owner-only (`chmod 600`) file **outside the repository** and explicitly
configure `DEVIN_SESSION_TOKEN_FILE`. Do not paste the token into chat or commit it.

```bash
cd backend
# No real credential needed; validates rejection, not subscription access:
DEVIN_LIVE_TEST=1 go test -tags=live ./internal/pkg/devin \
  -run '^TestDevinLiveUnauthenticated$' -v -count=1

# Discover the account's actual model IDs first (no inference request):
DEVIN_LIVE_TEST=1 DEVIN_SESSION_TOKEN_FILE=/absolute/path/to/private-token \
  go test -tags=live ./internal/pkg/devin \
  -run '^TestDevinLiveCatalog$' -v -count=1

# Paid/usage-consuming opt-in. Choose a model returned by discovery:
DEVIN_LIVE_TEST=1 DEVIN_SESSION_TOKEN_FILE=/absolute/path/to/private-token \
  DEVIN_LIVE_MODEL=swe-1-6-slow go test -tags=live ./internal/pkg/devin \
  -run '^TestDevinLiveSubscription$' -v -count=1
```

Authenticated checks discover models and validate non-streaming text, streaming
text/usage/termination, a function call, and a linked tool-result continuation.
They make at most **four short inference requests**, each with a 128-token output
limit, without automatic retries. Default model: `swe-1-6`; use
`DEVIN_LIVE_MODEL` to explicitly choose another model from your account's catalog.
Missing credentials or unsupported tool capability are reported as **skipped**,
not successful live validation. These checks exercise the native adapter; live
end-to-end gateway validation additionally needs a running sub2api URL and a
Devin-scoped test API key.
