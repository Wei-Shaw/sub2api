# Native Responses Namespace Compatibility

## Context

Codex clients can send private `namespace` tool declarations and tag historical
`function_call` input items with a `namespace`. The chat-completions fallback
bridge already flattens these declarations into ordinary function tools and
restores the namespace on responses. The native Responses OAuth path currently
forwards the private fields unchanged. Upstreams that implement only the public
Responses schema reject them, and sub2api surfaces the failure as a 502.

## Scope

Extend namespace compatibility to the native Responses OAuth path without
changing the existing chat-completions bridge or ordinary function tools.

The request transformation must:

- flatten every function child of a `type: "namespace"` tool into a top-level
  function tool using the existing collision-safe naming convention;
- rewrite namespace-qualified historical `function_call` items to the same
  flattened name and remove their private `namespace` field;
- rewrite namespace-qualified `tool_choice` values when present;
- reject ambiguous flattened names with a clear client error instead of
  forwarding a request that cannot be routed safely;
- preserve all unrelated request fields and tools.

The response transformation must:

- restore a flattened tool call to its original child name plus `namespace`;
- cover both JSON responses and every relevant SSE lifecycle event, including
  the final completed response;
- leave tool calls that are not in the request mapping unchanged.

## Design

Extract the namespace flattening and mapping behavior that is currently tied to
the chat-completions bridge into reusable helpers in `internal/pkg/apicompat`.
The helpers will operate on Responses request structures and expose the mapping
from flattened names to original namespace/name pairs.

The native OAuth forwarding path will apply the request helper during its
existing Codex OAuth normalization pass. The resulting mapping is request-local
and will be passed to the native response handling code. JSON and SSE response
rewriters will use it to restore function-call identity before bytes are sent to
the client. No mapping is persisted across requests because the request contains
the declarations needed to reconstruct it.

This change targets the native HTTP Responses forwarding path demonstrated by
the issue. WebSocket forwarding is outside the scope unless a failing regression
test proves that it shares the same HTTP transformation boundary.

## Error Handling

Malformed namespace declarations that cannot be transformed safely, including
flattened-name collisions, return a 400-class invalid-request response. Upstream
transport failures continue through the existing error and failover handling.
Response rewriting is conservative: unrecognized event shapes pass through
unchanged, while valid mapped function-call objects are restored.

## Testing

Tests will be written before implementation and will demonstrate the regression
against the current code. Coverage includes:

- OAuth native request forwarding with namespace tools and historical calls;
- namespace `tool_choice` rewriting;
- collision rejection;
- non-stream JSON response restoration;
- streaming SSE restoration across added, done, and completed events;
- ordinary function tools and payload fields remaining unchanged;
- targeted service and `apicompat` test suites, followed by formatting and the
  repository's relevant backend verification commands.

## Non-goals

- Changing the Codex client or requiring a client downgrade.
- Forcing OAuth accounts through the chat-completions fallback bridge.
- Changing connector security policy from issue #3408.
- Adding persistent namespace state or changing account scheduling behavior.
