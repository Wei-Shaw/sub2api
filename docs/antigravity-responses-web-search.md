# Antigravity Google Search with Responses function tools

Some Antigravity Gemini upstreams reject a request containing both native
`googleSearch` and `functionDeclarations`, even with
`includeServerSideToolInvocations=true` (see #6464).

For Gemini Responses requests with both tools, the gateway exposes a reserved
function to the model and executes it internally only when the model requests
search. Each search uses a separate request containing native Google Search
and no client functions. The original model candidate, call IDs, opaque thought
signatures and individual search results are retained for the continuation.
Client functions are never executed by the gateway. When a candidate contains
both search and client calls, the client calls receive explicit deferred
results and the model decides again after reading the search results.

Search-only requests continue to use native Google Search directly. Function-only
requests and Chat Completions keep their existing paths. This change does not
implement a `codeExecution` loop or change model discovery, aliases or pricing.
Search filters, offline-only search and user-location options are rejected on
this path instead of silently ignoring restrictions the bridge cannot enforce.

## Configuration

| Persisted setting | Environment fallback | Default |
| --- | --- | --- |
| `antigravity_agent_web_search_enabled` | `ANTIGRAVITY_AGENT_WEB_SEARCH_ENABLED` | `true` |
| `antigravity_agent_max_tool_rounds` | `ANTIGRAVITY_AGENT_MAX_TOOL_ROUNDS` | `8` |

A nonempty persisted setting overrides the environment. The round limit accepts
1–32; values above 32 are clamped, and invalid/nonpositive values use 8. Eight is
a gateway safety default, not a claim about the official client's loop limit.
The third consecutive identical search is returned to the model as a structured
tool error. At most 32 search attempts are allowed per request. Reaching the
round limit fails the request without emitting a partial Responses lifecycle.

Disabling the feature removes incompatible Google Search from mixed requests
without injecting the reserved function. Standalone native search remains
available. The reserved name `__sub2api_google_search` cannot also be used by a
client function in an enabled mixed request.

## Results and limits

Internal rounds are buffered. Successful grounded searches become separate
`web_search_call` items, followed by the final model output in one Responses
lifecycle with contiguous sequence numbers and output indexes. Search call
actions retain the query and sources. The answer includes a separate source-list
text part with URL citations over the actual source labels. Search-response
grounding offsets are **not** applied to unrelated generated answer text.

A search rejection is returned to the model as a structured tool error without
the upstream error body. Credential and service failures retain account failover
handling; another account starts from the original client request. Responses
without search grounding are not reported as successful searches. Cancellation,
idle timeout, malformed/truncated SSE and oversized responses abort collection.
Each upstream response is limited to 16 MiB and the accumulated request history
and rendered SSE to 64 MiB.

All decision, search and final usage is aggregated for the successful request,
with one existing gateway billing settlement. Internal buffering delays the
first client-visible event until the loop ends. No OAuth credentials, raw
official-client traces or diagnostic logs are needed in tests or PR attachments.
