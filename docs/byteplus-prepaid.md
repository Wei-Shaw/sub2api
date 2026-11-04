# Operator-managed BytePlus pack-only funding

Pack-only mode is **opt-in**, shared by enrolled provider credentials and account
aliases. Unenrolled credentials retain ordinary provider billing behavior. It
works with existing OpenAI API-key accounts using the official Ark base and with
native BytePlus accounts when that provider is installed.

Initial qualified policies cover the Johor/AP endpoint and these pinned models:

- `dreamina-seedance-2-5-260628`: standard 480p/720p/1080p video, 4–30 seconds.
- `dola-seed-2-1-turbo-260628`: text/function-call input with an inclusive output
  bound covering reasoning and visible output, using verified free-token quota.

EU and other model versions are not qualified for pack-only mode. Ordinary
unenrolled EU accounts continue to work. New billing versions require reviewed
bounds and tests; arbitrary administrator-entered prices are not accepted as
safety limits.

## Scope and prerequisites

This is a gateway admission/reservation ledger, **not** a BytePlus account-wide
billing switch. The separate read-only billing query below observes provider
balances without synchronizing them into funding. BytePlus can use
pay-as-you-go billing after applicable packs expire or run out. See its
[resource-pack rules](https://docs.byteplus.com/en/docs/modelark/seedance-2-0-model-resource-pack-rules).

Before crediting or resuming a pool:

1. Pause/drain earlier unguarded traffic and account for pre-existing queued jobs
   and unsettled charges. The gateway cannot retroactively reserve earlier work.
2. Verify each applicable pack's **current remaining tokens**, exact provider
   instance ID, model and expiry. Do not enter its original purchase amount,
   coupon USD balance or a price estimate. Provider reporting delays must be
   accounted for before accepting a balance snapshot.
3. Isolate every credential that can consume this funding, for example with the
   provider's IP restrictions covering only gateway egress addresses. Enroll all
   applicable keys in the same verified pool. Outside consumption invalidates the
   gateway's available-balance assumptions.
4. Keep server time synchronized. Preserve this guard in every gateway replica
   and subsequent release; an older unguarded replica cannot enforce its policy.

The feature does not buy, renew or top up packs, remove cards, disable provider
autopay, or prevent manual purchases/unrelated services. Protection depends on
accurate verified inputs, isolated consumption and the qualified billing policy.

## Admin setup without SQL

Edit a saved API-key account using
`https://ark.ap-southeast.bytepluses.com/api/v3` and open **BytePlus prepaid
funding**. The controls are separate from the account-edit form and use saved
credentials. Only full administrators may read or modify a shared pool; scoped
admin keys are rejected even if they can edit one member account.

1. Confirm the prerequisites and choose a new named pool or an existing paused
   pool. Enabling starts **paused**, with no inferred credit.
2. Add verified pack IDs, models, remaining balances and expiry. The balance unit
   selector accepts tokens, K tokens or M tokens with exact decimal conversion.
   Amounts must be representable in 1/64-token units. Pack IDs are globally
   unique; the known console display prefix is normalized so it cannot duplicate
   the same numeric instance ID.
3. Review the credited packs, then confirm and resume. No request is sent unless
   its complete conservative ceiling fits funding valid through its deadline.

The panel shows each pack's available, held and settled tokens, its funding
window, and unsettled jobs followed by recent history. Numeric available credit
can remain positive while a request is blocked by its size or expiry window.

### More accounts and replacement keys

Accounts using an enrolled key share its pool automatically. Account bindings
remain sticky across credential edits, including newly cloned accounts; changing
a key or endpoint cannot silently turn a protected account into PAYG.

Pause the pool before linking a replacement/additional key. Save the key in the
account, verify that it uses the same billing resources and region, and explicitly
link it to the existing pool. Old key aliases remain enrolled; counters are not
reset. A mistaken, paused pool with no grant or reservation history can be merged
into the intended pool. Funded pools are never silently merged.

Moving an account to another verified pool requires the old pool to be paused
with no unsettled jobs. Funds remain in their original pools.

### Reconciliation and opting out

**Check provider status** performs a read-only request for the task stored in a
reservation. The server, not the browser, decides terminal status and usage.
Repeated checks are idempotent. Provider media URLs and authentication material
are not returned by this budget API.

Queued/running jobs keep their holds. Missing usage, disconnects, timeouts,
ambiguous submission and elapsed deadlines do not refund funding. DELETE alone
is not evidence that a running task stopped. A submission with no task ID needs
investigation at the provider; there is no unsafe manual "refund" button.

Pause and settle all held jobs before verifying a pack balance. After use, a pack
can only be conservatively reduced or shortened, not refilled or extended.
Unused initial entries can be corrected before their first dispatch.

Returning to PAYG requires a separate **explicit PAYG acknowledgement** and zero
unsettled reservations. It preserves all financial history. Re-enabling starts
paused and requires fresh verification of every unexpired positive pack because
its balance may have changed while enforcement was off. Re-enrollment and duplicate
pack submissions never reset spent counters.

Usage above the qualified request ceiling safety-locks the pool. Normal resume
cannot clear that lock. Investigate the provider policy and reconcile the account
before making an audited repair; do not override it with a guessed refund.

## Transport and persistence

The required SQL store is checked at application startup. Missing store wiring or
schema fails startup instead of returning an unguarded transport. Each outbound
HTTP call makes a fresh indexed policy lookup; no TTL cache can hide enrollment,
key changes or endpoint changes on another replica. Runtime funding uncertainty
blocks dispatch. This makes the funding database an explicit availability
dependency of the shared upstream transport.

Upstream WebSockets are not metered by these policies and are refused for
enrolled keys before dialing and before every socket write, including pooled
connections established before enrollment. Unenrolled sockets retain their
ordinary behavior. Use the HTTP transport (or client WS HTTP-bridge mode) for
qualified text/task requests. A refusal is terminal rather than a retry/failover
to another paid transport.

Eligible grants are locked and allocated transactionally before dispatch. Integer
1/64-token units represent the published pack deduction ratios exactly. Model
bounds cover input and output, inclusive reasoning limits, supported dimensions,
frame rounding and additional headroom. Paid auxiliary features, explicit cache
storage, licensed assets, draft jobs and unqualified routes are rejected.

Video execution lifetime is forced to 3,600 seconds. Grants must survive that
lifetime plus a ten-minute margin. Text has a five-minute transport limit and the
same margin. Expired grants are excluded synchronously, without needing cron or
an open browser. A hold persists across process/database connections and past
its deadline until confirmed settlement.

The migration preserves balances, holds and safety locks from the earlier private
transport schema. Do not restore an old database snapshot or roll back to an
unguarded binary while continuing to serve protected accounts. Pause them and
reconcile provider consumption first.

## Admin API

Under `/api/v1/admin/accounts/{id}/byteplus-prepaid`:

- `GET /`: current pool, qualified policies, grants and recent/held jobs.
- `GET /pools`: full-admin shared-pool catalog.
- `POST /enable` or `/link`: `confirmed`, optional `pool_id` and `name`.
- `POST /grant`: `confirmed`, `pack_id`, `model`, decimal-string
  `remaining_tokens`, and RFC3339 `expires_at`.
- `POST /verify-grant`: `confirmed`, `pack_id`, decimal-string
  `remaining_tokens`, and RFC3339 `expires_at`.
- `POST /freeze`: pause admission without releasing holds.
- `POST /resume`: `confirmed`; requires eligible, freshly verified funding.
- `POST /disable`: `allow_pay_as_you_go`; refuses unsettled jobs/safety locks.
- `POST /reservations/{reservation_id}/reconcile`: read provider status and settle
  only known terminal usage/failure. No client-supplied usage or failure override.

Admin authentication, audit and compliance middleware still apply. Normal clients
use the existing inference/task APIs. Local guard denials retain structured
400/402/503 responses and cannot trigger an upstream failover.

## Validation and isolated browser fixture

Run the normal project checks, including the new critical frontend tests:

```sh
make test-frontend
cd backend
make test-unit
go test -race -tags=integration ./internal/pkg/packguard
go test -race -tags=integration ./internal/server/routes -run TestBytePlusPrepaidAdminGatewayIntegration
```

Integration tests provision disposable PostgreSQL with Docker unless an isolated
`PACK_GUARD_TEST_DSN` is supplied. They use fake credentials/vendor responses and
cannot incur provider charges. Coverage includes concurrent reservations and
enrollment, pack deduplication, aliases/key edits, queues/restarts/expiry, retained
ambiguous holds, final usage, scoped-admin rejection, opt-out/re-enable and legacy
schema upgrades.

For manual browser verification, run in separate terminals from the repository:

```sh
cd backend
PACK_GUARD_BROWSER_HARNESS=1 go test -tags=integration ./internal/server/routes -run '^TestBytePlusPrepaidBrowserHarness$' -v -timeout=150m
```

```sh
pnpm --dir frontend exec vite --config test/e2e/vite.prepaid.config.ts
```

Open `http://127.0.0.1:5198/test/e2e/byteplus-prepaid.html`. It renders the real
funding component against the real admin handlers and PostgreSQL, with synthetic
authentication and a deterministic vendor. It is not a production deployment or
real-provider test. The test-only submit/succeed/expire controls expose no real
funds, keys or payment method. The API listens on loopback by default; Docker
users can set `PACK_GUARD_BROWSER_BIND=0.0.0.0:18991` inside the container and must
publish it only to `127.0.0.1:18991`. Test files/pages are not production entries.

## Usage display and read-only billing queries

The Accounts usage cell recognizes native BytePlus accounts and legacy API-key
accounts with a validated official ModelArk URL. **Refresh local status** reads
one consistent SQL snapshot. It shows exact per-model spendable and held tokens,
per-pack expiry and recorded funding-verification times. Expired, paused,
safety-locked, mismatched and too-soon-expiring grants are not spendable. Account
USD limits and account expiry remain separate gateway settings.

**Query BytePlus** reads account money, resource packages and coupons through the
BytePlus Billing OpenAPI. It does not submit inference, add funding, release holds,
resume a pool, enable PAYG or change quotas. Each source has its own last-success
and last-attempt time. Failed/incomplete sources retain dated successful data;
missing data is not zero. Billing can lag usage. Package units and coupon
restrictions are retained; there is no combined money/token total or automatic
reconciliation. Cached observations are process-local and cleared by restart.

Billing access is separate from the ModelArk inference bearer key. Without billing
configuration, the button opens setup guidance and local refresh still works.
Only full administrators can access these shared account-wide observations.

### Server configuration

Mount a private regular JSON file with permission `0600` (or stricter) readable by
the service, set `BYTEPLUS_BILLING_CONFIG_FILE` to its path, and restart. Keep this
file outside the repository and ordinary account credentials. Example with fake
placeholders only:

```json
[
  {
    "pool_id": "operator-verified-pool-id",
    "account_id": "123456789",
    "access_key": "REPLACE_ON_SERVER",
    "secret_key": "REPLACE_ON_SERVER",
    "session_token": "",
    "region": "ap-southeast-1",
    "resource_types": ["Package", "RSC"]
  }
]
```

The configured owner must match `QueryBalanceAcct.AccountID`; mismatch blocks
asset display. Configure only the documented read permissions for
`QueryBalanceAcct`, `ListResourcePackages` and `ListCoupons`; do not grant purchase,
renewal, payment or policy-write permissions. Verify the exact IAM policy using
BytePlus's current console/API documentation for the target account. Temporary
AK/SK plus session token are supported; the operator must renew them before expiry
and restart to reload the file. Credentials never go to the browser or API output.

The client uses the documented global `https://open.byteplusapi.com/` billing
endpoint, service `billing`, version `2022-01-01`, signed with the configured
region. Package categories may be `Package`, `RSC`, or `RI`; defaults query Package
and RSC. Confirm which categories/product/configuration codes include this
account's ModelArk packs. The query does not claim all free/promotional grants
are returned. Compare package instance IDs, exact units, validity and coupon
restrictions with the BytePlus console before relying on coverage. Timestamps
without an explicit timezone are rejected rather than interpreted speculatively.

Queries coalesce per pool with a 30-second cooldown, a 30-second overall deadline,
15-second request timeout, maximum 20 pages per source/category and a 2 MiB
response limit. The client does not follow redirects or retry provider failures.
Pagination limits and duplicate/changing records produce an incomplete result,
not a false empty balance. Local auto-refresh follows the Accounts preference
and never invokes provider queries. Several application replicas each keep their
own observation cache and cooldown; configure provider rate limits accordingly.

References: [official billing SDK](https://github.com/byteplus-sdk/byteplus-go-sdk-v2/tree/main/service/billing),
[credentials](https://github.com/byteplus-sdk/byteplus-go-sdk-v2/blob/main/docs/1-Credentials.md),
[request structure](https://docs.byteplus.com/en/docs/byteplus-platform/reference-request-structure).
