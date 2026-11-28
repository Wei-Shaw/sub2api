# Batch Account Window Statistics

`POST /api/v1/admin/dashboard/account-stats/batch` is a read-only,
admin-authenticated endpoint for clients displaying account subscription cycles.
Each account supplies its own exact interval. It does not assume that all
accounts reset together or at midnight.

## Request

```json
{
  "windows": [
    {
      "account_id": 18,
      "start_at": "2026-09-21T12:34:56.123Z",
      "end_at": "2026-09-28T12:34:56.123Z"
    },
    {
      "account_id": 19,
      "start_at": "2026-09-24T08:30:00+08:00",
      "end_at": "2026-09-28T12:34:56.123Z"
    }
  ]
}
```

- 1 to 100 unique, positive account IDs per request.
- RFC3339 timestamps, including fractional seconds and explicit offsets.
- Half-open intervals: `start_at <= created_at < end_at`.
- Both bounds are required; each interval must be positive and at most 31 days.
- The body is capped at 64 KiB; unknown fields and trailing JSON are rejected.
- Invalid batches fail before querying the database. Duplicate IDs are rejected,
  not silently merged. Query work has a 30-second deadline, bounded further by
  an earlier caller deadline.

## Response

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "stats": {
      "18": {
        "requests": 2,
        "tokens": 300,
        "cost": 1.5,
        "standard_cost": 3,
        "user_cost": 4
      },
      "19": {
        "requests": 0,
        "tokens": 0,
        "cost": 0,
        "standard_cost": 0,
        "user_cost": 0
      }
    }
  }
}
```

The statistics reuse the existing `AccountStats` contract:

| Field | Meaning |
| --- | --- |
| `requests` | Number of usage log rows in the interval |
| `tokens` | Input + output + cache creation + cache read tokens |
| `cost` | Sum of `COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)` |
| `standard_cost` | Sum of `total_cost` |
| `user_cost` | Sum of `actual_cost` |

IDs with no matching usage logs (including unknown IDs) return explicit zero
statistics, consistent with the existing account-filtered model query. The
endpoint does not perform a separate account existence check. It reads retained
usage logs, not archived lifetime totals.

Errors fail the whole batch; partial results are never returned as zero values.
The response uses `Cache-Control: no-store`. No database schema, pricing,
scheduling, or account configuration changes are required.

`cost`, `standard_cost`, `requests`, and `tokens` match the summed model endpoint
for the same interval. Do not copy its account-filtered `actual_cost` into
`user_cost`: the model endpoint aliases that field to account cost when an
account filter is supplied; this endpoint explicitly retains the real user cost.

This endpoint complements the existing account batch method that accepts one
shared window start. Use this endpoint when each account has its own reset
boundary, such as subscriptions activated at different times or rolling
windows evaluated from account-specific metadata.

Clients should key any local cache by account ID and the exact window start.
When supporting mixed server versions, only fall back to an older endpoint
after confirming that this route is unsupported (404/405); do not turn
timeouts, authentication failures, or server errors into a per-account fan-out.

## Query Design and Verification

A typed, parameterized `VALUES` relation supplies account-specific intervals.
A lateral aggregate uses each account and time range as index conditions. The
whole batch executes in one PostgreSQL statement and snapshot, with one database
round trip. There is no per-account HTTP or SQL call loop.

Validate with unit tests for the handler, service, interval contract, and SQL
repository, then PostgreSQL integration tests for exact boundaries, distinct
account windows, empty accounts, pricing fallbacks, and equivalence to existing
model totals. Performance measurements must separate local query timing from
production end-to-end latency; fewer round trips do not imply that scanning
retained usage rows becomes free.
