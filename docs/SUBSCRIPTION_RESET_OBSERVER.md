# Subscription reset observation

An administrator can select a reference set of ordinary OpenAI OAuth accounts
for a subscription group and observe whether their Codex weekly quotas reset
together. The subscription management page exposes this setting after selecting
a specific OpenAI subscription group.

This first stage is observation only. Saving a policy, reaching quorum, or
restarting the worker does not clear subscription usage, extend subscriptions,
change configured limits, or consume upstream reset credits. The selected
daily/weekly/monthly dimensions describe the intended future action.

## Configuration

- Mode: `off` (default) or `observe`.
- Source: `7d`. The actual upstream duration must be 604800 seconds.
- Reference accounts: 1–200 explicitly selected accounts in this group. Spark,
  shadow and agent-identity scopes are excluded.
- Quorum: 80% by default, configurable from 1–100%.
- Aggregation window: 10 minutes by default, configurable from 1–60 minutes.
- Intended reset dimensions: daily, weekly and monthly, initially all selected.
- Single-subject mode: opt-in; requires two separate fresh observations.

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

Early percentage drops or unexplained boundary changes are retained as
`needs_review`; they do not become confirmed natural resets just because many
accounts exhibit them. Local reset-credit markers exclude affected observations.
Identity or plan changes require reviewing/re-saving the reference policy.

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
  "version": 0
}
```

Status includes each reference account's evidence state, distinct subject count,
the active candidate's frozen denominator/quorum, and up to 100 recent events.
The default history response contains 20 events.

## Planned execution semantics

An eventual automatic action must publish one idempotent group event and protect
in-flight request accounting. It must not repeatedly call the existing manual
reset endpoint whenever an individual account resets.

The intended supplemental reset keeps subscription expiry unchanged: daily
usage clears while retaining the next midnight refresh; selected weekly/monthly
windows restart from the action's effective time. Selecting all dimensions on
every upstream weekly reset therefore also replenishes the monthly allowance
approximately weekly. This is distinct from replacing all local reset schedules
with an upstream schedule.
