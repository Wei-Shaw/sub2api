# Billing and usage

Two endpoints let a client answer, without opening the dashboard, *"what will
this cost me"* and *"what have I spent"*.

## Rate multipliers

```bash
curl {{SITE_ORIGIN}}/v1/sub2api/billing \
  -H "Authorization: Bearer $API_KEY"
```

```json
{
  "object": "sub2api.key_billing",
  "schema_version": 1,
  "billing_scope": "token",
  "group_rate_multiplier": 1.0,
  "resolved_rate_multiplier": 0.8,
  "peak_rate_enabled": true,
  "peak_start": "09:00",
  "peak_end": "18:00",
  "peak_rate_multiplier": 1.5,
  "applied_peak_multiplier": 1.5,
  "effective_rate_multiplier": 1.2,
  "timezone": "Asia/Shanghai",
  "observed_at": "2026-08-14T10:30:00Z"
}
```

Read it from the bottom up — `effective_rate_multiplier` is the number that
actually prices your next request. The rest explains how it was reached:

| Field | Meaning |
| --- | --- |
| `billing_scope` | What is metered. `token` means token-based pricing. |
| `group_rate_multiplier` | The base multiplier of your key's group. |
| `user_rate_multiplier` | Present only when you have a personal override. |
| `resolved_rate_multiplier` | The multiplier after any personal override. |
| `peak_rate_enabled` | Whether the group prices peak hours differently. |
| `peak_start`, `peak_end`, `timezone` | The peak window, in the group's timezone. |
| `peak_rate_multiplier` | The peak surcharge factor. |
| `applied_peak_multiplier` | Present only while you are inside the window. |
| `effective_rate_multiplier` | Resolved × applied peak. What you pay now. |

Optional fields are omitted, not nulled. `user_rate_multiplier` absent means you
have no override; `applied_peak_multiplier` absent means the current moment is
off-peak.

Responses carry `Cache-Control: no-store`, since the effective rate changes when
a peak window opens or closes. Read it when you need it rather than caching it
for the day.

Two cases return an error instead:

- `403` `permission_error` — the key has no group, so no rate can be resolved.
- `404` `not_found_error` — the deployment runs in simple mode, which has no
  billing model at all.

## Consumption

```bash
curl "{{SITE_ORIGIN}}/v1/usage?days=7" \
  -H "Authorization: Bearer $API_KEY"
```

Scoped to the calling key. `days` is optional and must be between 1 and 90; a
value outside that range returns `400`:

```json
{
  "type": "error",
  "error": {
    "type": "invalid_request_error",
    "message": "Invalid days, allowed range is 1-90"
  }
}
```

The response carries the key's totals plus a per-day breakdown. Usage statistics
are collected best-effort: if the statistics store is briefly unavailable the
endpoint still answers with the basic fields rather than failing the request, so
treat a missing breakdown as *"not available right now"*, not as zero.

## What is not billed

- `POST /v1/messages/count_tokens` — checks your quota and subscription, records
  no usage, and consumes no concurrency slot.
- Polling an async image task or a video status. The work was billed when you
  submitted it; reading the result is free.

## Running out

An exhausted quota answers `429`. On OpenAI-compatible paths it uses OpenAI's
own shape, so an SDK's retry logic recognises it:

```json
{
  "error": {
    "message": "...",
    "type": "insufficient_quota",
    "param": null,
    "code": "insufficient_quota"
  }
}
```

Top up, or ask the operator to raise the limit. Retrying will not help until one
of those happens — see [Errors](/docs/errors).
