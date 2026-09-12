# Subscription reset observation and automatic replenishment

An administrator can select a reference set of ordinary OpenAI OAuth accounts
for a subscription group and observe whether their Codex weekly quotas reset
together. The subscription management page exposes this setting after selecting
a specific OpenAI subscription group. A confirmed batch can replenish the
group's active subscriptions once when the administrator enables auto mode.

The default mode is off. Observe mode records evidence without changing usage.
Saving or changing a policy starts a fresh observation baseline and never grants
quota immediately. Neither mode changes configured dollar limits, extends
subscription expiry, nor consumes upstream reset credits.

## Configuration

- Mode: `off` (default), `observe`, or `auto`.
- Source: `7d`. The actual upstream duration must be 604800 seconds.
- Reference accounts: 1–200 explicitly selected accounts in this group. Spark,
  shadow and agent-identity scopes are excluded.
- Quorum: 80% by default, configurable from 1–100%.
- Aggregation window: 10 minutes by default, configurable from 1–60 minutes.
- Reset dimensions: daily, weekly and monthly, initially all selected.
- Single-subject mode: opt-in; requires two separate fresh observations.
- Early reset confirmation: opt-in, disabled by default.

Team/Pro names can help select references, but do not establish shared quota
identity. Votes are deduplicated by upstream user + account/workspace + quota
scope. Raw upstream identity strings are hashed before entering observer state.
Unknown or offline members remain in the frozen denominator. Exhausted accounts
are monitored even when they are temporarily unavailable to the scheduler.

For example, 8 confirmed resets among 10 independent reference subjects meet an
80% quorum once. The remaining 2 confirmations update the same event. Other
accounts in the routing group do not vote unless selected as references.

## Evidence and polling

The worker makes lightweight GET usage queries independently of client traffic.
It does not probe model inference or request reset-credit details. A query is
shared by all policies referencing the same local account. Each 30-second scan
has a 25-second deadline, a maximum of 40 account queries, and at least 500 ms
between queries. Successful samples are normally refreshed every 2 minutes;
within 10 minutes of the expected reset they are refreshed every 30 seconds.
Failures back off, up to 16 minutes. A cross-instance leader lock prevents a
healthy cluster from multiplying the normal scan rate.

These are bounded best-effort intervals, not detection latency guarantees. Large
account sets, slow upstream responses and rate limiting can delay confirmation;
the UI exposes stale/unknown evidence. Narrow aggregation windows require enough
fresh samples to arrive within that window.

Natural reset confirmation requires a fresh observation after the old boundary
with a consistent next weekly window. Second-level reset timestamp jitter is
ignored. Exact 0% is unnecessary. Missing/null percentages, expired countdowns,
failed queries and history rows finalized by a timer are not reset evidence.

Early percentage drops remain `needs_review` unless early confirmation is
enabled. An early transition requires usage of at least 20% followed by at most
5%, then another fresh sample at most 5% at least 30 seconds later. The quota
identity, plan and new reset boundary must stay stable. Each distinct reference
subject must satisfy this evidence rule before its vote counts toward quorum.
The thresholds are heuristics, not proof of an official provider event; changes
to provider quota capacity can also lower a usage percentage.

Local reset-credit markers exclude affected observations. Identity or plan
changes require reviewing/re-saving the reference policy. Repeated early resets
use a chain of preceding observed transitions, so late members of an older batch
cannot join a newer batch just because their reset timestamps match. Ambiguous
or truncated evidence stays `needs_review`. Natural reset identities remain
available while a fresh baseline can still report their boundary. Timed-out
events are not reopened to grant another allowance.

Policies use optimistic versions. Reconfiguration starts a new baseline and
invalidates in-flight samples from the old version. Observer state and changed
audit events are persisted in the same transaction under a per-policy row lock.
This consumer does not share the quota-history ingester's `processed_at` cursor.

## Admin API

All routes use the existing administrator authentication boundary:

```text
GET /api/v1/admin/groups/:id/subscription-reset-policy
PUT /api/v1/admin/groups/:id/subscription-reset-policy
GET /api/v1/admin/groups/:id/subscription-reset-status?limit=20
```

The `PUT` body includes the last read `version`; a concurrent change returns
409 and requires reloading. The group ID in the route is authoritative.

```json
{
  "mode": "observe",
  "source": "7d",
  "account_ids": [11, 12, 13],
  "quorum_percent": 80,
  "aggregation_minutes": 10,
  "reset_dimensions": ["daily", "weekly", "monthly"],
  "allow_single_subject": false,
  "allow_early_resets": false,
  "version": 0
}
```

Status includes each reference account's evidence state, distinct subject count,
the active candidate's frozen denominator/quorum, and up to 100 recent events.
The default history response contains 20 events.

## Execution semantics

The executor consumes confirmed events only for the current auto policy version
and an active OpenAI subscription group. Confirmations older than the configured
aggregation interval are marked `execution_expired`; a restart does not catch up
by issuing historical grants. The worker checks every 15 seconds with a
12-second work budget and a cross-instance leader lock. Database idempotency
also protects concurrent/repeated delivery independently of that lock.

The supplemental reset keeps subscription expiry unchanged: daily
usage clears while retaining the next midnight refresh; selected weekly/monthly
windows restart from the action's effective time. Selecting all dimensions on
every upstream weekly reset therefore also replenishes the monthly allowance
approximately weekly. This is distinct from replacing all local reset schedules
with an upstream schedule.

Only subscriptions that are active, started, unexpired, not deleted, and have
activated usage windows are affected. Future, suspended and unactivated
subscriptions do not receive a historical grant. Existing first-use activation
and one-day subscription rules remain in force. The event audit records the
confirmation kind, execution time, dimensions, affected subscription count and
group revision.

Publication uses set-based SQL over eligible subscriptions in one transaction.
This is O(N) database work, not a per-user HTTP or service loop. The selected
usage buckets, compatibility counters, execution record and durable outbox row
commit together. The unique event ID makes a repeated delivery a no-op. An
outbox row is an atomic publication record; admission does not depend on an
asynchronous cache notification consumer.

## In-flight billing and subscription lifecycle

Final request admission and group publication acquire shared/exclusive locks on
the same database group row. The effective timestamp is sampled from the
database after the exclusive lock is acquired and becomes visible on commit;
it is not a prediction of the exact commit instant.

Each admitted request keeps immutable daily/weekly/monthly bucket IDs. HTTP
admission refreshes after account-slot waits, and WebSocket admission refreshes
for each logical turn. Failover within a turn preserves its admitted IDs.
Asynchronous billing settles into those IDs even if replenishment happened in
the meantime. Selected dimensions charge their historical buckets; unchanged
dimensions still increase their matching current counters. Billing deduplication
and bucket charges share the existing billing transaction.

Managed admission uses authoritative database state and ignores the legacy
Redis subscription counters. Delayed billing never applies an unversioned delta
to a newly replenished subscription. Lifecycle operations, manual resets and
normal window maintenance use the same bucket model. An expired renewal starts
a new term; an ordinary extension preserves current usage. Old admitted charges
remain attributable after renewal or revocation. This does not introduce strict
concurrent quota reservation, so existing concurrent overspend remains possible.

## Rollout and storage

Migrations 242/243 add the quota tables and automatic execution support. Applying
them does not enable any group. On a group's first auto-mode save, bucket
accounting is initialized in the policy transaction with the existing counters;
this initialization does not replenish usage. Later selecting off or observe
stops future automatic grants but keeps bucket accounting enabled.

Deploy the new version to every serving instance and drain all in-flight
requests and queued billing before a group's first auto-mode enablement.
New-version requests carry database-clock legacy tokens that preserve original
window anchors across conversion, but the old counter model cannot distinguish
a same-anchor manual reset before enablement (for example, two daily windows
starting at the same midnight). The first-enable drain also covers that case;
do not rely on legacy tokens as a universal drain-free migration mechanism.
Old binaries cannot supply tokens at all. Missing tokens for managed groups fail
closed; mixed-version operation and rollback to an old binary after enablement
are not supported by this draft. Subsequent resets and auto/off mode switches
keep bucket accounting and do not require this first-enable drain.

Historical usage buckets and their charge records are retained for delayed
settlement and attribution. This draft has no automatic retention job for those
tables or reset audit/outbox records; operators should account for their storage
growth. Never delete referenced buckets while an admitted request may still
settle into them.

The set-based group transaction is covered by concurrency and rollback tests,
but has not been load-tested with very large subscription groups.

The first release supports the upstream 7d source only. The 5h source, multiple
cohorts controlling one dimension, capacity-weighted votes and an upstream-only
replacement for local refresh schedules remain separate work. API-equivalent
dollar estimates from quota history are not votes and do not set subscribers'
billed cost or allowance. Observe real reset batches before choosing production
quorum/aggregation settings; the default 80%/10-minute values are initial policy
defaults rather than measured guarantees.
