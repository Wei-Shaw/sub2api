# OpenAI Built-in Tools Augmentation Design

## Background

Current OpenAI-compatible forwarding already supports normal `function` tools and, after recent fixes, can preserve built-in `web_search` when users explicitly send it.

However, some downstream clients want a simpler compatibility layer: a private request parameter that tells the gateway to **augment** the outgoing OpenAI request with stable built-in tools, without forcing users to rewrite their original tool list and without overriding existing user-specified tools.

Recent production investigation also established a firm boundary:

- `web_search` is currently the only built-in tool that is stably usable on this line.
- `image_generation` is not currently entering the effective upstream tool set.
- `code_interpreter` is unsupported on this line.

Therefore phase 1 must target `web_search` only.

## Goal

Add a body-top-level private parameter `builtin_tools` that can augment OpenAI requests with built-in tools, while preserving original request semantics.

Phase 1 only supports appending:

```json
{"type":"web_search"}
```

## Non-goals

- No support for `image_generation`, `code_interpreter`, `file_search`, or other built-ins in phase 1
- No sticky/session/ops/dashboard changes
- No automatic `tool_choice` rewriting
- No frontend/admin UI changes in this phase

## Supported request forms

The private parameter lives at the **top level of the request body**.

Phase 1 accepts these forms:

1. Boolean convenience form:

```json
"builtin_tools": true
```

2. List form:

```json
"builtin_tools": ["web_search"]
```

3. Explicit object form:

```json
"builtin_tools": {"web_search": true}
```

Normalization is deterministic in phase 1:

- only `web_search` is recognized
- all other built-in names are ignored, not treated as errors
- duplicate `web_search` requests collapse to one effective built-in
- object values only count when they are explicit boolean `true`

Examples:

- `true` => append `web_search`
- `["web_search"]` => append `web_search`
- `["web_search", "code_interpreter"]` => append only `web_search`
- `{ "web_search": true }` => append `web_search`
- `{ "web_search": true, "code_interpreter": true }` => append only `web_search`
- if the original request already contains any tool with `type == "web_search"`, no duplicate is added

## Scope

Phase 1 only applies to these OpenAI-compatible HTTP request paths:

- `/v1/responses`
- `/v1/chat/completions -> responses` compatibility chain

Explicit exclusions in phase 1:

- `openai_passthrough` accounts / passthrough forwarding path
- `/v1/responses/compact` and related compact suffix paths
- WebSocket / realtime transport paths

Reason: the goal is a minimal built-in augmentation layer on the standard OpenAI HTTP mainline, without pulling local passthrough/compact branches into phase 1.

Clarification:

- the exclusion applies to **client-facing WS / realtime entry paths**
- normal HTTP `/v1/responses` requests remain in scope even if the internal upstream transport later resolves to Responses WSv2
- in other words, phase 1 is defined by the client-facing request contract, not by the eventual internal transport selected by the gateway

## Merge / precedence semantics

`builtin_tools` is an **augmentation layer**, not a replacement layer.

Rules:

1. Original request `tools` are always preserved.
2. Gateway first normalizes `builtin_tools` into a built-in tool list.
3. Gateway then appends only missing built-ins.
4. If the request already contains `{"type":"web_search"}`, no duplicate is added.
5. Existing `function` tools are never modified or removed.

So the result is:

- original tools remain intact
- built-in augmentation is additive only

For `/v1/chat/completions -> responses`, the **augmented** tool set is also the one that should define the final compatibility request semantics. In particular, any compat logic that derives stable request identity from the tool set must observe the post-augmentation result rather than the raw pre-augmentation request.

## `tool_choice` semantics

The gateway must **not** silently rewrite `tool_choice` because of `builtin_tools`.

Rules:

1. If the client already sent `tool_choice`, preserve current compatibility behavior.
2. Do not auto-change it to `required`.
3. Do not inject a built-in-specific `tool_choice` automatically.
4. `builtin_tools` only changes the final `tools` set.

This avoids changing user intent unexpectedly.

## Upstream request hygiene

`builtin_tools` is a local/private parameter and must never be forwarded upstream.

Before building the upstream OpenAI request body:

- parse `builtin_tools`
- augment `tools`
- remove `builtin_tools` from the body

This requirement applies to both `/v1/responses` and the `chat/completions -> responses` compatibility path.

## Implementation boundary

Backend-only minimal scope:

- `backend/internal/pkg/apicompat/chatcompletions_to_responses.go`
- `backend/internal/pkg/apicompat/types.go`
- `backend/internal/service/openai_gateway_chat_completions.go`
- `backend/internal/service/openai_gateway_service.go`
- optionally one small helper file if it keeps the parsing/merge logic isolated and easier to test

Do not expand into sticky/session/ops/dashboard code in this feature.

Implementation note for chat compat:

- this cannot be implemented only inside `chatcompletions_to_responses.go`
- the chat-compatible request path must parse/strip `builtin_tools` before the final Responses request body is produced
- the private field must never leak into the upstream OpenAI request body

## Testing strategy

Required regression coverage:

1. `builtin_tools: true` appends `web_search`
2. `builtin_tools: ["web_search"]` appends `web_search`
3. `builtin_tools: { web_search: true }` appends `web_search`
4. existing `web_search` is not duplicated
5. existing `function` tools remain untouched
6. `builtin_tools` is removed from the final upstream request body
7. `tool_choice` is preserved as-is
8. `/v1/chat/completions -> responses` compatibility path also applies augmentation correctly
9. `openai_passthrough` path does **not** apply augmentation and does not leak `builtin_tools`
10. `/v1/responses/compact` and related compact suffix paths do **not** apply augmentation

Validation commands:

- targeted backend tests
- `go test ./internal/handler ./internal/repository ./internal/server/... -count=1`
- `go test -tags unit ./internal/service ./internal/pkg/apicompat -count=1`
- `go build ./cmd/server`

## Risk notes

- The biggest risk is accidental duplication or accidental `tool_choice` mutation.
- Keeping the parsing/merge logic isolated in a helper reduces the risk of silently changing unrelated OpenAI request behavior.
- Restricting phase 1 to `web_search` prevents us from pretending unstable built-ins are currently supported.

## Phase 1 acceptance criteria

Phase 1 is successful when:

- downstream clients can opt into built-in `web_search` using `builtin_tools`
- existing explicit `tools` requests continue to behave the same
- no duplicate `web_search` entries are produced
- no unexpected `tool_choice` mutation happens
- the private parameter never leaks to upstream
