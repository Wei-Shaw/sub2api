# Structured output compatibility

The adapter accepts Responses requests using text.format.type=json_object or
json_schema. BPS rejects native text/response_format request fields, so the
adapter sends the output requirements as developer instructions and validates
the final response locally. This does not provide upstream constrained decoding.

- Preserve the requested model and account. Model-access rejections remain
  upstream errors; structured output support does not grant model permissions.
- Return a successful final answer only when it is valid JSON and, for
  json_schema, satisfies the supplied schema. Do not retry, repair, strip fences,
  or replace invalid model output with fabricated data.
- Withhold structured message text until the terminal response is validated.
  Reconstruct message events from that validated response, rather than replaying
  unvalidated deltas. Ordinary text requests remain incremental.
- Preserve tool continuations and explicit refusals as protocol items, outside
  the final-answer JSON contract. Preserve upstream failure/incomplete status
  without exposing partial structured message text.
- Resolve schema references inside the submitted document only. Never fetch
  remote schemas or read local files. Limit schemas to 1 MiB and answer text to
  16 MiB, subject to the existing SSE event limit.

# Tool history and input boundaries

- OAuth models routed through BPS do not advertise native encrypted multi-agent
  capabilities. Apply this limit to generated, fetched and pinned group catalogs
  by the actual mapped route, and recalculate the client ETag. Plaintext client
  tools remain available; refresh the catalog and start a new conversation to
  leave an old client-side v2 override or encrypted history behind.
- Rebuild complete CUSTOM calls using their current catalog's exact
  `codex2api.custom/NAME` marker and raw input. Preserve cached native calls
  verbatim. Retain unavailable historical tools as recorded history; never
  infer a new tool target from executable text.
- Require a matching complete call or scoped replay-cache entry for each tool
  result. Report the input path when neither is available. Do not invent calls
  or discard their results.
- Reject encrypted message parts with an `encrypted_content` diagnostic and
  input path. Do not reinterpret ciphertext as plaintext. Top-level encrypted
  reasoning items retain their existing handling.
- The OAuth gateway retries once when HTTP 400 specifically rejects encrypted
  content and the prepared history can recover by removing opaque reasoning
  items only. It keeps the same account, model, effort, proxy, attachments and
  conversation metadata, and closes the rejected response before retrying.
  It never drops encrypted messages, tool results or compaction context. A
  second rejection returns `invalid_encrypted_content` with plaintext-history
  recovery guidance; ordinary 400s and transport failures do not use this retry.
- Preserve `detail: original` on HTTPS images and inline images rewritten by
  the relay. Let the upstream model validate its supported detail levels; do
  not silently downgrade the requested detail.
- Send only type and file_id for validated message attachment references,
  including uploaded images, replayed IDs and images moved from tool results.
  Inline tool screenshots retain their existing detail handling.
- Enforce the saved inline-image count and byte limits; defaults remain 20
  inline images and 32 MiB per request for the relay.
  A relay capacity error and an upstream overload are separate from a tool
  protocol error; HTTP 200 alone does not establish a successful SSE terminal.

# Tool transport corrections

- Validate a complete native tool batch before committing replay entries or
  emitting any client tool events. Never evaluate transport code in the gateway.
- When a completed batch consists entirely of identifiable run_officejs calls
  with valid outer arguments, return local validation errors as native tool
  results with `executed: false`. Continue on the same BPS account, model, effort,
  proxy and scoped conversation, with at most two corrective model requests.
- Preserve intended operations and call order. Require the corrected batch to
  have the same number of calls and pass the current catalog's transport checks.
  Reject changes to previously valid operations. For an invalid raw call corrected
  to an explicit CUSTOM, FUNCTION_CODE or FUNCTION_CMD transport, bind its code field to the
  original bytes before checking the operation and size limits. Never dispatch
  source text rewritten by the correction model, including whitespace changes.
  Store the bound native call in replay history so later turns see the exact
  operation dispatched to the client. Do not bind named JSON envelopes or infer
  targets from source text; retain the explicit target and whole-batch checks.
  Keep ordinary text incremental and retain its original response identity and
  output indexes. Hide intermediate correction text and native tool events.
- Release the preceding response body before a correction request so accounts
  with concurrency one do not deadlock. Cancel corrections on client disconnect.
  Include all returned terminal usage in the final success or failure response.
- This formatting path does not retry transport I/O failures, upstream rejections, incomplete streams, undeclared tool targets, unsupported native tools, missing call identities, function argument schema failures or structured answer failures. Preserve any explicit declared target on correction.
  Do not infer a tool target from arbitrary raw code. Stop on a changed batch or
  exhausted correction limit without dispatching any client tools.
- Treat this as bounded model correction, not constrained upstream decoding or
  a guarantee that arbitrary malformed model output will always recover.

# First-turn unknown-target correction

The gateway separately permits one regeneration when the first tool interaction ends in exactly one undeclared run_officejs target at the end of a completed response. It must have no prior tool calls/results and no dispatched client tool. This path reuses the prepared request, current catalog, account, model, proxy and attachment IDs; it does not append a fabricated executed tool result. Its corrected response must pass the current catalog, argument schema, identity and parallel-call checks. A second unknown target, invalid arguments or incomplete response fails without dispatching tools. Both attempts' reported usage is retained, including progressive usage if the correction disconnects before its terminal event.

This path and the existing known-target formatting path are selected independently. A function argument schema error alone does not trigger either path.

For streaming clients, first-turn regeneration stops once nonempty text, reasoning
summary or refusal content has been forwarded. An unknown target then produces
`response.failed` with the original response identity and usage, without tool
dispatch or another generation. Empty lifecycle events and opaque reasoning alone
do not block regeneration; non-streaming requests remain buffered and retain their
one correction attempt. Known-target formatting corrections still preserve the
original text and replace only withheld tool slots.

# Optional inline image limit policies

Administrator settings under Facilities → Feature switches → Excel / BPS image
support select off (default), automatic compaction, or warning interception.
Existing persisted settings need no migration. Clients omitting the new fields
preserve their current values. Native uploads and HTTPS relay use the same
inline image counting rules, including tool outputs and agent messages; repeated
image occurrences each count. Existing size and resource limits still apply.

Automatic compaction only runs when history plus new images exceeds the configured
limit and each partition fits independently. It accepts Codex client identities
and a stable session scoped by account, API key, thread and model. A digest of the
last successful input identifies the unconsumed tail; bootstrap accepts only a
trailing user batch or a complete terminal tool call/result batch. Uncertain
boundaries fail explicitly. Old history is compacted at most once with client
tools disabled, and the actual encrypted compaction window is used for the
continuation. New inputs stay intact. The compacted window is emitted before
continuation items with adjusted output indexes, and both phases' reported usage
is counted even when generation fails. This consumes additional model tokens.
The client must retain and echo the compaction output items on later requests;
the gateway reconciles verified history checkpoints as described below. Mock
protocol tests do not replace acceptance testing in the actual Codex client;
unsupported clients should use manual compact.

Warning mode requires 1 <= reserve < warning remainder < maximum images. With
20/8/3, 0–11 pass, 12–17 warn once per conversation cycle then pass on retry, and
18–20 block ordinary requests. At 12 images the user sees 5 available slots.
Only administrators see the reserve setting. Explicit compact endpoints and native
compaction triggers bypass these warning thresholds, but not the total limit.
Successful compact or a return below the warning threshold starts a new cycle.
Redis stores only progress position/digest and the atomic warning marker, expiring
after two idle hours. A missing stable session identity rejects requests requiring a policy action.
Redis session-state errors fail closed for ordinary requests; manual compact
remains available when Redis policy state is unavailable.

## Gateway checkpoint reconciliation

Ordinary Codex compaction output does not itself replace local client history.
After automatic compaction, the gateway commits an authenticated checkpoint
before emitting the window. Redis stores only canonical SHA256 digests, input
positions and split positions; it never stores the compacted window or full
messages. The client must echo the exact emitted window immediately after the
request input. On subsequent requests the gateway verifies the prior input and
window, moves the preserved new-input tail after that window, and drops only
the verified compacted prefix from the BPS request. It applies at most 16 such
checkpoints in order. Altered/missing windows fail explicitly where the prefix
matches; unrecognized histories retain normal limits. Concurrent checkpoint
writes use compare-and-swap and abort on conflict before generation.

The client's raw upload can still include old images. Raw request-size protection
remains in force; this is gateway reconciliation, not client memory cleanup.
A manual compact resets the checkpoint chain. State expiry or a different account,
model, key or thread can require manual compact again. Checkpoints remain useful
if generation fails after emitting the compaction window; all usage is still billed.

# Configurable image capacity

Administrators can raise image limits for larger servers without changing the
conservative defaults or existing saved settings. The settings UI, admin API,
per-request settings reader and admission middleware use these ceilings:

| Setting | Default | Maximum |
| --- | ---: | ---: |
| Request body | 64 MiB | 1024 MiB |
| Shared admission budget | 1024 MiB | 65536 MiB |
| In-flight requests | 128 | 4096 |
| Relay image size | 20 MiB | 512 MiB |
| Relay image bytes per request | 32 MiB | 512 MiB |
| Images per request | 20 | 65536 |
| Relay disk storage | 1024 MiB | 262144 MiB |
| Relay stored images | 512 | 1048576 |
| Relay link lifetime | 30 minutes | 10080 minutes |

The budget must cover eight times the configured body limit, and its effective
value remains at least eight MiB per in-flight slot. It is accounting capacity,
not preallocated RAM. Storage must cover one request's image bytes and count;
request image bytes must cover one image. Base64 expands encoded request bodies.
The server/global body limit, gateway body limit and reverse proxy can impose
smaller limits; to admit bodies above their defaults, adjust them explicitly.
Higher settings do not bypass those limits or provision memory or disk.

Native attachments retain their 20 MiB per-image and 32 MiB per-request size
limits and their bounded attachment-ID cache. The per-request image count is
configurable in both modes. Both modes retain the 64-megapixel safeguard.
Relay download concurrency and automatic compaction checkpoint limits are
unchanged. Large capacity settings require appropriate resource provisioning;
these ceilings are not throughput or memory-use guarantees.

# In-band upstream failures

- Recognize error, response.failed and response.cancelled as failed terminals.
  Explicit error status takes precedence over known error codes and types;
  unknown identifiers use a safe 502 fallback. Never forward upstream free text.
- Buffered client responses return the classified HTTP status. Started streams
  retain their HTTP status and carry the classification in the error event.
  Ops records keep the actual upstream HTTP status separate from the semantic
  failure status; existing compact keepalives still finish with an SSE failure.
- A rate-limit terminal cools only the BPS route for subsequent requests, except
  observation-only quality probes. It never retries the accepted generation,
  disables an account on an in-band 403, or changes shared Codex account health.
- Failure/cancellation retains usage and withholds unvalidated tools. Readers
  stop at cancellation without waiting for EOF, including correction and image
  compaction readers. This does not add a BPS WebSocket transport.
