# Conversation logs (opt-in)

Conversation logs let administrators inspect the latest user prompt, model reply,
tool calls, and request context for HTTP requests passing through this instance.
They are separate from billing/usage logs and cannot recover historical content
that was not captured.

## Enable and inspect

1. Apply the normal application migrations and start the updated server/frontend.
2. Sign in as an administrator and open **Conversation logs** in the sidebar
   (`/admin/conversation-logs`). Existing administrator authentication and
   deployment-compliance checks still apply.
3. Review your privacy notice, access policy, storage budget, and applicable
   consent requirements **before** enabling capture.
4. Set retention and the per-side capture limit, enable capture, and explicitly
   save. The installation default is **disabled**, seven days, and 1 MiB each for
   the request and response. The API accepts 1–365 days and 16 KiB–2 MiB per side.
5. Send a request through a supported endpoint. Search by text/request ID, user
   ID, model, outcome, or time range. Open a record to inspect context, export a
   JSON copy, or permanently delete that record.

The **Redact sensitive fields** checkbox controls credential-field masking in
future saved request/reply bodies and tool calls. It defaults to enabled. Disable
it and save to retain fields such as `api_key`, `password`, and `access_token` as
supplied in the body. Authorization HTTP headers are still never captured;
inline binary data and invalid/truncated JSON remain omitted. A record's
`redacted` flag describes its capture mode, so changing the current setting does
not relabel or rewrite earlier records. Values already replaced by `[redacted]`
cannot be recovered. Each in-flight request keeps the mode selected at its start.

The configuration API exposes `redact_sensitive`. Updates from older clients that
omit this field preserve the currently configured mode.

Disabling capture affects subsequent requests; in-flight captures can still
finish. Existing records remain readable until explicitly deleted or expired.
Retention cleanup removes up to 1,000 expired records per minute. It is eventual,
not an exact expiry-time guarantee. Normal backups and exported copies are not
purged by record deletion or retention cleanup.

## Protocol coverage and outcomes

Supported POST routes are `/v1/chat/completions`, `/chat/completions`,
`/v1/responses`, `/responses`, `/backend-api/codex/responses`, `/v1/messages`,
`/messages`, and `/antigravity/v1/messages`. JSON and SSE are supported, including
fragmented UTF-8 output and tool-call deltas. Captures reflect the **downstream
client-visible response**, after any gateway protocol conversion.

WebSockets, native Gemini endpoints, image/audio endpoints, admin traffic, and
requests without an authenticated API-key identity are not persisted. The
normalized summary is not a full-fidelity protocol transcript: it selects the
first Chat Completions choice and client-visible text/tool calls, not hidden
reasoning. Sanitized captured JSON/SSE data is available separately in details.

Outcomes distinguish `completed`, `failed`, `interrupted`, `incomplete`, and
`capture_truncated`. Captures reaching either size limit carry explicit request
and response truncation flags. Invalid or truncated JSON is replaced by a note
instead of storing unparsed bytes; this can leave the prompt summary empty.

## Privacy and access

- Content is sensitive and is stored as ordinary PostgreSQL text/JSON, **not
  application-level encrypted data**. Use appropriate database, backup, and
  transport protections. User labels/key names are snapshots stored with records.
- Authorization headers are not captured. When redaction is enabled, known structured credential fields
  (`authorization`, `api_key`, `access_token`, `refresh_token`, `password`, and
  `client_secret`) are redacted recursively. Inline `data` strings and data URLs
  are omitted. This is **not** a general secret/PII detector: secrets written in
  natural-language prompts, model replies, arbitrary strings, or JSON-encoded
  tool arguments may remain. Do not enable capture for traffic you cannot retain.
- Full context and replies are available only through administrator routes.
  List responses contain short prompt/reply previews, not full bodies. Content
  reads are registered as sensitive audit events and responses use `no-store`.
- The UI renders content as text, not executable HTML. There is no public
  conversation-sharing link or ordinary-user history API in this change.

## Runtime and storage behavior

Capture buffers are bounded independently on request and response. At most 32
requests are captured concurrently, with a separate 32-record asynchronous write
queue. Saturation skips capture instead of delaying inference. Writes have a
two-second timeout; graceful shutdown allows eight seconds for queue draining.
Recording is best-effort, **not a durable audit/compliance journal**. The admin
view exposes saved, skipped, failed-write, queued, and active-capture counters;
these counters reset on process restart. Parsing and sanitization happen after
the handler finishes, while streamed bytes and flushes are forwarded immediately.

Configuration is persisted in the singleton `conversation_log_config` row and
records in `conversation_logs`; no Redis content store or billing-table changes
are introduced. Migrations `248_conversation_logs.sql` and
`249_conversation_log_redaction.sql` are additive. Existing records are marked
redacted by the new column's default. Removing the
feature from a deployment does not automatically delete retained content.

Configuration changes update the serving process's atomic snapshot. **Multiple
application replicas do not automatically refresh each other's configuration.**
For now, use a single recording instance or coordinate restarts/configuration on
every replica; do not assume changing one replica disables all replicas.

## Reproducible checks

From `backend/`:

```sh
go test -race -count=1 ./internal/conversationlog
go test ./internal/server/routes ./internal/server/middleware -run 'Test.*(ScopedAdmin|AdminAPIKey|Conversation|Audit|Gateway)'
```

From `frontend/`, after installing the lockfile dependencies:

```sh
pnpm exec vitest run src/features/conversation-logs/ConversationLogsView.spec.ts
pnpm run build
```

Tests cover transparent JSON/SSE forwarding, early stream delivery, credential
redaction, disabled/unauthenticated capture, truncation, cancellation, protocol
parsing, saturation, asynchronous writes, shutdown draining, configuration/filter
validation, and safe text rendering in the administrator UI.
