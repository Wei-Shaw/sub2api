# Account scheduling pools

Scheduling pools organize OpenAI OAuth accounts without changing their existing routing or billing groups. The optional scheduling policy maintains a fixed active set of Free accounts for each configured shared exit. It is disabled by default.

## Setup

1. Pause incoming traffic and wait for existing requests to drain. Requests started while the policy was disabled have no pool lease; enabling it cannot retrospectively account for those streams. Apply the same precaution when re-enabling a previously disabled policy.
2. Open **Accounts → Account pools**. Under **Scheduling policy**, set the maximum active accounts per exit (default: three). Give direct access and every proxy using the same public exit the same exit-group name. Unmapped routes all share `default`. Separate proxy records do not imply separate public IP addresses.
3. Enable the global policy before enabling additional reserve accounts or importing new schedulable accounts.
4. Create a pool, add selected accounts, and enable the pool. JSON account-data import and CRS import can also attach new accounts to a pool as part of account creation.
5. Enable the manual scheduling switch for accounts that may participate, then resume traffic. A pool never changes an account's manual scheduling switch; standby status is separate from that switch.

The exit limit includes accounts in other pools and accounts outside pools. Known paid accounts remain visible for management but do not consume the Free-account budget. Accounts with missing or unrecognized plan metadata are governed conservatively; an account without a stable upstream identity cannot be activated.

## Rotation

The active set persists in PostgreSQL and is shared by all application instances. An active account stops accepting new requests when any recorded quota window reaches 100%. The window's actual duration and reset timestamp are used, including a 30-day window. Existing requests retain their exit capacity until they finish; only then may a standby account take the slot. Temporary throttling and ordinary concurrency saturation do not activate extra reserve accounts.

Quota recovery places an account at the back of the standby queue. It does not displace an already active account. Administratively disabled accounts stay disabled, and an invalid credential must be repaired before the account becomes eligible again. A burst of three recognized permanent authentication failures on one exit pauses new reserve activation for ten minutes.

WebSocket connections are checked for each turn. An idle connection cannot keep sending after its account loses eligibility; the client must reconnect. HTTP streams and active WebSocket turns hold renewable request leases. A failed lease renewal cancels the upstream request rather than allowing it to continue without accounted capacity.

Reducing the configured limit lets existing requests drain. The old requests may temporarily exceed the new lower limit, but no additional account is admitted into that occupied capacity. Exit-group mappings cannot be edited while any request leases remain active: wait for requests to drain before saving the new mapping. This prevents a renamed group from granting another budget to the same actual exit. Changing an account's proxy also waits for that identity's existing activity to drain before moving it to its new exit.

## Imports and naming

The JSON entry point is the multi-account **Import** action on the Accounts page. Selecting a pool attaches compatible imported accounts atomically; a new account is never visible between its creation and its pool assignment. Non-OpenAI accounts in mixed imports keep their normal import behavior. CRS updates preserve local names, manual scheduling switches, and existing pool membership.

The optional import naming rule formats newly created accounts with a prefix, starting number, zero padding, and suffix. Existing accounts updated by CRS retain their local names when preserving local management settings (the UI default); pool members always preserve those settings. General bulk renaming of existing accounts is delivered independently and does not require scheduling pools.

## Operational boundaries

This policy limits business-request scheduling. OAuth refreshes and quota probes continue independently, including for standby accounts. It does not promise that only three identities ever contact upstream, nor does it establish that an upstream authentication failure was caused by the number of scheduled accounts.

Live/WebRTC calls have a separate lifecycle that can outlive their initial HTTP response and the gateway observer. They are not supported for governed Free or unknown-tier accounts while the policy is enabled. These requests are rejected before creating an upstream call. Known paid accounts and API-key Live calls retain their existing behavior.

Exit groups are administrator-supplied mappings, not public-IP detection. Keep them accurate when proxies share an egress, switch upstream routes, or rotate public IP addresses. The policy relies on upstream quota observations already stored on accounts; it cannot predict quota exhaustion before upstream reports it.

PostgreSQL admission is serialized across instances and fails closed if it cannot establish capacity. This favors a predictable limit over availability during database failures. Request leases expire after 90 seconds and renew every 20 seconds. All instances serving the installation must run the updated code before enabling the policy.

Each admission scans OpenAI OAuth identity and availability metadata under a shared PostgreSQL advisory lock; access and refresh tokens are not loaded. Candidate filtering has a short cache, but final admission always reads current database state. The implementation has concurrency coverage with a local PostgreSQL database; large-installation throughput has not been benchmarked.
