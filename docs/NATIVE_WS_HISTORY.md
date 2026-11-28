# Bounded native WebSocket failover history

## Scope

Native Responses `ctx_pool` ingress retains the current context window inside
one live client connection. It does not persist messages in PostgreSQL/Redis,
share history across clients, or recover history after a process restart.
HTTP/SSE, HTTP bridge and passthrough transport/history implementations are
unchanged. HTTP bridge already retains account-failover input and rebuilds the
current turn; this change supplies the missing native WS history. The shared
WS failover handler refreshes current
request routing/billing metadata, including existing HTTP-bridge retries.
Account ranking, queue fairness, slot ownership and retry limits are unchanged.

## State and replay

1. An explicit root request replaces history. A continuation is eligible only
   when its `previous_response_id` matches the last completely observed response.
2. Retain input and successful terminal `response.output`, including assistant
   text, reasoning/encrypted content, native tool calls and tool outputs. The
   terminal array supplies the authoritative output ordering, even if streamed
   `output_item.done` events arrived in another order.
3. Append client deltas once per turn. A complete prefix resent by the client
   is not appended again. Existing immutable history bodies are shared; newly
   received bodies are detached from large network buffers once.
4. On an already-eligible later-turn upstream failover, rebuild the CURRENT
   request with full current-window input and without the old account's
   `previous_response_id`. Use the current original model and unscoped identity
   source, then let the next account apply its own mapping/identity rules.
   Refresh the handler's selection and billing model for that request as well.
5. A repeated failure uses this reconstructed current request, never the
   connection's first frame. Existing switch limits and output-visible guards
   still apply. This does not promise exactly-once execution after ambiguous
   upstream failures or portability of account-specific encrypted data.

A new root after remote compaction discards the old window. A changed window
that still supplies an old continuation anchor is not trusted as a complete
root. Unknown chains, unresolved `item_reference` entries, incomplete/missing
terminal outputs, non-object shorthand input (rather than Codex's structured
message arrays), missing tool-call pairs and exhausted budgets disable
transparent migration. The client must reconnect/resubmit a complete request
(1013 for an exhausted 429 failover). Healthy forwarding continues when the
cache is unavailable; requests are not silently truncated or converted to user
messages. Existing known-invalid encrypted-content filtering is unchanged.

## Limits

Under `gateway.openai_ws`:

| Setting | Default |
| --- | ---: |
| `native_replay_max_session_bytes` | 67108864 (64 MiB) |
| `native_replay_max_items` | 32768 per session |

Any zero disables this cache; negative values are rejected. Environment names
are uppercase with underscores, e.g.
`GATEWAY_OPENAI_WS_NATIVE_REPLAY_MAX_SESSION_BYTES`.

There is no aggregate cache admission limit. Usage is accounted across
connections, but another session cannot exhaust a shared
history quota. Limits are not preallocated: memory grows with actual history.

Reservations count JSON body lengths plus 64 bytes per item. They bound the new
retained history, NOT process RSS: allocator rounding, existing tool-only replay
state, client frames, collector output, request serialization and Go GC remain
additional costs. Old bodies are not copied on every delta; full failover JSON
serialization is paid only when migrating. Closing the connection/attempt or
discarding a window releases its reservation. Exceeding either per-connection
limit releases the entire cache; a later complete root within both limits can
resume caching. One abnormal connection cannot retain unlimited history, but
many large connections can still exhaust process memory. Size deployment
capacity and ingress concurrency accordingly; this is not a process-wide OOM
guarantee.

## Tests

- Service tests: complete history, immutable ownership, full-prefix dedup,
  compaction, repeated rebuild, missing anchors/items/outputs, exact per-session
  boundaries, output overflow, item limits and concurrent accounting/cleanup.
- Real local WebSocket integration through the gateway handler: three accounts,
  two consecutive 429 failures, same-window continuation, window 67 to 68,
  model changes, reordered streamed output, fail-closed budget/unknown-chain
  cases, and no transparent replay after downstream-visible output.
- Benchmarks: 128-turn input-replay maintenance before/after adding the native
  history cache, with 1/10 KiB input/output bodies; 4 MiB failover serialization.
- Opt-in memory pressure tests: `SUB2_HISTORY_MEMORY_TEST=1`, all 64 sessions
  retain independent 4 MiB roots, with/without the additional cache; one session
  grows past 60 MiB and discards history when its next delta exceeds 64 MiB.

Benchmarks are synthetic local tests, not live OpenAI quota/latency tests. Do not
interpret cache-only timings as end-to-end gateway throughput improvements.
