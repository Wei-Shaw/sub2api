# Account-level Codex STATE tickets

Codex STATE acquisition is opt-in per non-shadow OpenAI OAuth account. The gateway setting remains the master switch and stores the shared harvest proxy; enabling that switch alone does not enable any account.

## Configure an account

1. In **Settings → Gateway → Codex**, enable STATE tickets and configure the shared dynamic proxy URL.
2. Save a fixed business proxy for the account, then reopen **Accounts → Edit**.
3. In **STATE tickets**, select **Pro (292)** or **Team (332)**, enable the account and save. This is a manual setting; it does not read or modify the subscription.
4. Wait for a usable ticket. The panel shows acquisition time, expiration, renewal/cooldown state and watchdog events, and provides a manual reacquisition button. It never displays the STATE blob.

The account API accepts `ticket_plan`, `enabled` and `model` through `PUT /api/v1/admin/accounts/:id/codex-ticket`; `GET` returns the summary and `POST /api/v1/admin/accounts/:id/codex-ticket/harvest` starts acquisition. Supported targets are `gpt-6-astra` (UI default) and `gpt-5.6-sol`.

A proxy username containing `{sid}` receives a new random SID on each attempt. Existing 1024proxy `-sid-...-t-N` usernames are also supported. Different SIDs do not guarantee different egress addresses. If reaching the harvest proxy itself requires an outer HTTP CONNECT proxy, configure `gateway.openai_codex_ticket.harvest_dial_proxy_url` at startup; this does not change business traffic.

## Acquisition and retention

A candidate is acquired using the same account through the shared dynamic proxy, then replayed through that account's saved business proxy. Both probes must complete with the requested model in the actual response, and the candidate must match the manually selected length. A fixed-proxy replay returning a valid 312-byte STATE is rejected.

Verified tickets are persisted before becoming usable in memory. Their local lifetime is one hour, with renewal ten minutes before expiry. Each round has at most eight attempts and a five-minute failure cooldown. HTTP 401, 403 or 429 stops the round immediately. A failed renewal retains a still-valid ticket without extending its original expiration. Restart restores only unexpired tickets with matching account/configuration bindings.

Missing tickets block only the enabled account's configured target model. Disabled accounts and other models retain their existing routing. Tickets are not shared between accounts.

## Response watchdog

For requests that actually used the current ticket, a completed response with a different model or a successful HTTP response with a valid 312-byte STATE invalidates that ticket and schedules reacquisition. The 312 length is an experimental refresh signal, not an upstream-specified revocation protocol. Delayed responses cannot invalidate a newer ticket, and concurrent signals reuse one running acquisition job.

The observer preserves response bytes and does not replay business requests. HTTP SSE/JSON observation is bounded to 1 MiB per event/body; incomplete, malformed or oversized content is not inferred to be a model mismatch. Enabled accounts use the existing WebSocket HTTP bridge so the same validation applies; other accounts retain their transport policy.

## Upgrade behavior

- This changes global gating to explicit account opt-in. Existing, newly created, imported and copied accounts default to disabled. After upgrading, deliberately enable the accounts that need tickets.
- Keep the global master switch and shared harvest proxy configured. Old account-specific proxy overrides are ignored; they are not automatically promoted to the shared pool.
- `target_length`, `models`, `fail_closed`, `ttl_seconds` and `refresh_before_seconds` remain parseable for configuration compatibility but no longer control account-level behavior. Length comes from the manual plan; lifetime and renewal use the policy above. Missing tickets fail closed only for an opted-in account's selected model.
- A plan/model/enable change invalidates old bindings and cancels stale jobs. Changing the shared pool keeps a valid ticket and its original expiry, but prevents an in-flight old-pool job from publishing results.
- State/configuration/watchdog summaries use private `accounts.extra` fields. Account reads/exports omit ticket material; general edits cannot inject or copy it. Audit logging redacts the shared proxy. Database backups still contain sensitive material.
- No database migration is added. Reverting application code does not remove account extra fields or secrets from the database.

## Limitations and design references

Ticket length and response model fields are routing observations, not model-quality measurements or guarantees of upstream acceptance. Pro/Team selection, recovery and revocation tests use synthetic responses. Upstream expiry and real revocation are not controllable test fixtures.

Acquisition synchronization is process-local; multiple replicas can acquire duplicate tickets. Checking fresh account configuration adds database reads to the request path. Watchdog invalidation uses a bounded five-second database context.

Built on the original ticket work in [#7315](https://github.com/Wei-Shaw/sub2api/pull/7315). Thanks to [gylive/ccodex-sleep-state](https://github.com/gylive/ccodex-sleep-state/tree/26b22196bf68b372d0daad9381f686a3321068d4) for lifecycle/status presentation ideas and community members for feedback. No code or dependencies from that reference project are included.
