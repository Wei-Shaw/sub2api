# Notification Webhook

> 中文版本：[NOTIFICATION_WEBHOOK_CN.md](NOTIFICATION_WEBHOOK_CN.md)

Deliver system notifications to your own HTTP endpoint so your own service can consume them. Email and webhook are independent channels that can be switched on or off per event — including turning email off entirely and running webhook-only.

Where to configure: Admin → Settings → Email → **Notification channels / Webhook**.

## Channel resolution

The effective channels for an event are decided by three layers:

1. **Defaults**: email on, webhook off. Behaviour is unchanged after an upgrade.
2. **Global switch**: `webhook.enabled` is a **hard off-gate** — while it is off no event delivers a webhook, so you never have to unset them one by one. Turning it **on does not start delivery for any event**; each event still has to be enabled explicitly.
3. **Per-event overrides**: the `email` / `webhook` switches, plus an optional dedicated endpoint URL and body template. Both endpoint fields inherit the global value when unset; the signing secret is always the global one. The `email` / `webhook` switches, when unset, fall back to the defaults in point 1 (on / off) and are unaffected by the global switch.

An event also needs a non-empty endpoint URL before its webhook does anything.

## Default payload

`POST`, `Content-Type: application/json; charset=utf-8`:

```json
{
  "schema_version": 1,
  "event": "content_moderation.violation_notice",
  "event_label": "Risk control violation notice",
  "category": "risk_control",
  "audience": "user",
  "locale": "zh",
  "site_name": "Sub2API",
  "delivery_id": "9f86d081...",
  "occurred_at": "2026-07-26T06:39:58Z",
  "timestamp": "2026-07-26T06:40:00Z",
  "recipient": {
    "user_id": 1234,
    "username": "alice",
    "email": "alice@example.com"
  },
  "source": { "type": "content_moderation", "id": "99" },
  "data": { "moderation_category": "...", "violation_count": "3" }
}
```

- When `audience` is `user`, `recipient` is what your consumer uses to address the right user. When it is `admin` the notification is operational, and `recipient.email` may be empty (delivery still happens after every admin mailbox has been removed).
- `schema_version` versions this envelope. `event` is the discriminator for the event-defined `data` shape; consumers should use structured fields rather than a generated display string.
- The default payload never includes an email template, rendered email content, or HTML. `occurred_at` is the source-event time; `timestamp` is when this delivery body was built.

### `content_moderation.cyber_policy_notice` data

Cyber-policy notifications contain the fields from the existing notification and upstream diagnostics only; they do not contain the user's original input:

```json
{
  "triggered_at": "2026-07-26T06:39:58Z",
  "model": "gpt-5.6-sol",
  "group_name": "default",
  "upstream_message": "This content was flagged ..."
}
```

`triggered_at`, `model`, `group_name`, and `upstream_message` come from the existing Cyber notification. `upstream_message` is a redacted and length-limited diagnostic string that may contain upstream response text or usage information; consumers must treat it as opaque and must not parse user input from it.

### `ops.alert` data

For `ops.alert`, `data` is the existing source DTOs, not an email-shaped projection:

```json
{
  "rule": { "id": 45, "name": "high error rate", "window_minutes": 5 },
  "alert": { "id": 123, "severity": "P1", "status": "firing", "metric_value": 6.91, "threshold_value": 5.0, "dimensions": { "platform": "openai", "group_id": 12 } }
}
```

`source.id` is the canonical envelope identity for the alert event; `data.alert.id` is the same ID in the typed source object. `dimensions` and each of its keys are optional. Metric values may be `null` when unavailable. Alert severities use the raw rule vocabulary (`P0`, `P1`, and so on); this is distinct from the notification-floor vocabulary (`critical`, `warning`, `info`).

`data.rule` and `data.alert` are the complete DTOs used by the Ops API, not only the fields shown in the example; fields may be added with later versions. At notification time `alert.email_sent` is `false`, because it represents successful email delivery only. `rule.notify_email` is likewise email-only and may be `false` on a webhook delivery.

### `ops.scheduled_report` data

Scheduled reports are computed aggregates, so `data` carries the report run metadata and the raw aggregate used to produce it, never the email HTML:

```json
{
  "report": {
    "name": "Daily summary",
    "type": "daily_summary",
    "schedule": "0 9 * * *",
    "start_time": "2026-07-26T09:00:00Z",
    "end_time": "2026-07-27T09:00:00Z"
  },
  "overview": { "request_count_total": 1234, "sla": 0.9995 }
}
```

Exactly one aggregate is present according to `report.type`: `overview` is the existing `OpsDashboardOverview` for daily/weekly summaries, `error_digest` is `{ "total": 42 }` for error digests, and `account_availability` is the existing `OpsAccountAvailability` for account-health reports. `account_availability` uses `group`, `accounts`, and `collected_at` in snake case; `accounts` is an object keyed by the account ID as a decimal string. Error digests intentionally do not include individual request logs, user identities, or client IPs. Use `report.start_time` and `report.end_time` to query detailed errors through the Ops error-log API when needed. The numeric DTO values stay numeric; email-only fields such as `report_html`, CSS display controls, placeholder `"-"` values, and percentage-formatted strings are not sent.

## Custom body template

Set a template and it replaces the default payload, so you can emit whatever shape your receiver expects:

```json
{"event":"{{event}}","source_id":"{{source_id}}","occurred_at":"{{occurred_at}}","rule":"{{rule_name}}"}
```

- Available placeholders are that event's email placeholders plus the webhook-only ones:
  `event` `event_label` `event_category` `audience` `locale` `user_id` `source_type` `source_id` `occurred_at` `timestamp`
- Values are substituted with JSON string escaping, so quotes, backslashes and newlines are safe.
- Saving validates that every placeholder belongs to the event and that the template renders to **valid JSON**. A global template is validated against every event, so it may only use placeholders all events share.
- A custom template is an explicit alternative body and can reference the event's template variables. It never receives email-template output or sample preview values. `rendered_title` and `rendered_text` remain temporarily valid for stored configurations but always render as empty strings; replace them with structured fields.

## Signature verification

Every delivery is signed. The secret is generated automatically when webhook
delivery is enabled — copy it from the settings page into your receiver. Every
request carries:

| Header | Meaning |
| --- | --- |
| `X-Sub2Api-Signature` | `hex(HMAC-SHA256(secret, timestamp + "." + body))` |
| `X-Sub2Api-Timestamp` | Unix seconds |
| `X-Sub2Api-Event` | Event name |
| `X-Sub2Api-Delivery` | Delivery ID, usable for consumer-side deduplication |

Deliveries are always `POST` with a JSON body. There is no method choice and no custom-header support — the signature is the authentication mechanism.

The target URL must be `http` or `https`. Response bodies are always discarded: nothing the receiver returns reaches a caller, a log, or the admin UI.

## Delivery semantics

- **Best effort**: delivered from a background goroutine; it never blocks the request that produced the notification.
- **Retries**: 2 by default with exponential backoff (1s, 2s). **An explicit 0 means never retry.** 5xx and 429 are retried; other 4xx are not, since resending an identical body is pointless.
- **Concurrency cap**: a slot is reserved non-blockingly **before** the delivery goroutine is spawned, so in-flight goroutines and HTTP requests are both capped at 32. When slots are exhausted the delivery is **dropped with a warn log** (carrying `event`, `delivery_id` and `reason=slots_exhausted` for log-aggregation alerting) — it never queues and never blocks the producing request.
- **Failures are dropped**: the persisted delivery marker is only written on success, and a failure is **not** rescheduled. `email_sent` retains its original meaning: it is set only after an email is actually sent; a successful or queued Webhook never changes it. Monitor the logs above if you need at-least-once visibility.
- **Retry budget is per event for admin notifications**: all three admin fan-out paths (ops alerts `ops.alert`, account quota `account.quota_alert`, scheduled reports `ops.scheduled_report`) dispatch one webhook up front and then run an **email-only** mailbox loop. One event therefore produces at most `max_retries + 1` HTTP requests regardless of how many admin mailboxes are configured, and a failed first dispatch is not restarted once per recipient.
- **Scheduled reports can be webhook-only**: an empty report recipient list is no longer skipped — as long as the webhook for `ops.scheduled_report` is enabled, the report is delivered. With no mailbox there is no recipient locale to resolve, so the envelope locale is the default (`en`); the per-mailbox emails still use each recipient's own locale.
  > Note: "reports enabled" (`report.enabled`) in the ops settings is the **master switch for the report job**, not an email-channel switch — turning it off stops the webhook too. To run webhook-only, leave it enabled and turn off the email channel for `ops.scheduled_report` on this page.
- **Deduplication**: the delivery key includes the channel, so a successful email never marks the webhook as delivered. Admin events are deduplicated per event rather than per recipient — three configured admin mailboxes still produce one push, not three. User events are deduplicated per recipient, one each.
- **Best effort, with possible loss and duplication**: the deduplication above is **per instance** (an in-process in-flight claim plus the persisted marker). A failed delivery or exhausted concurrency slot may drop an event, and across replicas the same event may be pushed more than once. **Your receiver must deduplicate on `X-Sub2Api-Delivery`**.
- **Redirects are not followed**: a 3xx is treated as a failure.

Ops alert webhooks are **not gated by the email recipient list or the email rate limit**: an exhausted mail quota does not affect webhook delivery. The two channels are independent.

## Event catalogue

| Event | Audience | Sent when |
| --- | --- | --- |
| `auth.verify_code` | user | Registration / email binding / OAuth completion / TOTP verification |
| `auth.password_reset` | user | Password reset link requested |
| `notification_email.verify_code` | user | Verifying an extra notification mailbox |
| `subscription.purchase_success` | user | Subscription purchased or renewed |
| `subscription.expiry_reminder` | user | Subscription about to expire (unsubscribable) |
| `balance.low` | user | Balance crossed the configured threshold (unsubscribable) |
| `balance.recharge_success` | user | Top-up fulfilled |
| `account.quota_alert` | admin | Upstream account quota threshold crossed |
| `content_moderation.violation_notice` | user | A request tripped risk-control rules |
| `content_moderation.account_disabled` | user | Risk control disabled the account automatically |
| `content_moderation.cyber_policy_notice` | user | Blocked by cyber-security policy |
| `ops.alert` | admin | An ops alert rule fired |
| `ops.scheduled_report` | admin | Scheduled operational report |

## Semantic changes to be aware of

- **Unsubscribing applies to every channel.** Once a user unsubscribes from an optional event the webhook stops too — unsubscribing means "stop notifying me about this", not "stop emailing me".
- **Ops alert rule mail switch remains mail-only.** An alert rule's `notify_email`, the alert recipient list, rate limit, and `Alert.Enabled` affect only email. The central `ops.alert` Webhook switch applies to every enabled rule that passes the shared notification severity floor and is not silenced, including rules with `notify_email=false`. The severity floor and silence suppress delivery on both channels; they do not stop alert-event creation or alert-history visibility.
