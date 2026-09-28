# Native Meta Muse runtime foundation

Tracking: [#7625](https://github.com/Wei-Shaw/sub2api/issues/7625).

This implements persistent workspace and accepted-turn ownership for the consumer
Muse app provider. It is a backend foundation, not a working inference provider.
No Muse account type, public route, model listing, browser runtime, or live
transport is enabled by this change. The app authentication and event contract
must be verified before those surfaces are connected.

## Implemented behavior

- `internal/pkg/muse` defines validated runtime inputs, a conservative turn state
  machine, bounded leases, and explicit flat-request/free-test pricing snapshots.
- `MuseRuntimeService` allocates server-generated operation IDs and persists the
  `submitting` state before a future transport may perform an external action.
- `NewMuseRuntimeRepository` uses the existing raw PostgreSQL repository pattern
  used by audit/plugin storage. Migration `241_muse_runtime.sql` adds canonical
  workspaces, verified local-account aliases, and durable turn records.
- The same remote principal/workspace has one downstream owner. Local account
  aliases share its active-turn boundary. Reservation checks the authenticated
  key owner, active user/account, selected provider, alias membership, and the
  account snapshot used during verification.
- A verification observation carries the selected account's `updated_at` value.
  An edit invalidates that observation until revalidation. This conservative
  fence includes non-credential account edits; it does not claim a verified Muse
  credential-version contract.
- Admission is checked again before `reserved → submitting`. A changed account
  cannot dispatch from a stale queued reservation.
- Expired leases do not clear remote occupancy. A recovery claimant obtains the
  same operation with a higher fence; an interrupted submission becomes
  `ambiguous`, never a fresh submission. Old workers cannot renew or advance it.
- Client cancellation does not release the remote workspace. Confirmed terminal
  outcomes clear occupancy; `owner_review` stops automatic recovery and remains
  occupied pending a future explicit owner-resolution flow.
- Turn reads enforce user and API-key ownership. Provider correlation IDs cannot
  be replaced by a different task ID after assignment. Prices are stored with
  the turn rather than recomputed from a mutable caller object.
- The complete nested `muse_session` credential document is classified as
  sensitive for DTO/audit redaction and preserved on redacted account edits.

## Integration boundary

`BindVerifiedWorkspace` accepts server-trusted identity information from a future
authenticated provider observation. It must not be exposed directly to a client
as a way to claim arbitrary remote identities. A binding also requires an active
`muse` account row with the exact observed account timestamp.

The future gateway must retain the workspace lease separately from HTTP request
slots and renew it while it owns work. Before external submission it must commit
`BeginSubmission`. After any uncertainty it must reconcile the original task.
Neither local lease expiry nor a lack of text output authorizes replay. Fencing
protects local persistence; it cannot undo remote work already accepted by Meta.

This foundation deliberately does not infer session cookie requirements, OAuth
grants, quota windows, VM trust modes, cancellation acknowledgements, causal
event fields, client tool support, or token counts. Those remain live-protocol
qualification requirements. Settlement and idempotent usage projection are
subsequent native components; a stored price snapshot alone does not apply a
charge or prove customer billing.

## Verification

```sh
cd backend
go test ./internal/pkg/muse
go test -race -tags=integration ./internal/pkg/muse
go test -tags=unit ./internal/service ./internal/handler/dto \
  -run 'Test.*Muse|Test.*Redact|TestMergePreservingSensitiveCreds|TestAudit.*Sensitive'
```

The integration test starts an isolated PostgreSQL container by default. An
explicit `SUB2API_MUSE_TEST_DSN` PostgreSQL URL can instead point to a disposable
test server. The test creates and removes only its own randomly named schema,
uses minimal existing-table fixtures, and applies the migration twice. It tests
concurrent aliases/replicas, lease expiry, recovery/fencing, cancellation,
ownership/admission, account changes, immutable pricing, provider correlation,
and the owner-review stop. It makes no Meta requests.

Existing providers and public gateway behavior remain unchanged. The migration
is additive; rollback of application code preserves these records for later
reconciliation rather than deleting accepted-work history.
