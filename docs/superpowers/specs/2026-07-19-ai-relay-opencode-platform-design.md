# AI Relay OpenCode Platform Design

## Goal

Add an `OpenCode` platform to the standalone AI Relay plugin. It accepts both OpenAI chat payloads and OpenCode-native chat payloads, then forwards an OpenAI-compatible request to an OpenCode upstream.

## Platform Behavior

- Platform key: `opencode`.
- Display name: `OpenCode`.
- Default Base URL: `https://opencode.ai/zen`.
- Authentication remains the incoming Bearer token.
- OpenCode upstream URLs automatically include `/v1` when the configured Base URL does not already end with `/v1`.
- Route-level mappings remain relative to the effective Base URL, including the automatic `/v1` segment.
- Responses, status codes, headers, JSON bodies, and SSE streams remain transparent.

## Request Conversion

Only JSON requests to `chat/completions` are candidates for conversion. Standard OpenAI payloads containing a `messages` array remain unchanged.

An OpenCode-native payload is converted when it contains usable `parts` or `message` content:

- A string `model` is trimmed and retained.
- An object `model` uses the first string value from `modelID`, `model_id`, or `id`.
- Non-empty `system` becomes a system message.
- Text from string parts or object `text`/`content` fields is trimmed and joined with newline characters into one user message.
- If no usable parts exist, non-empty string `message` becomes the user message.
- `stream`, `temperature`, `top_p`, `max_tokens`, and `stop` are preserved when present.
- Invalid JSON or payloads without convertible content are forwarded unchanged.

## Architecture

Register an `OpenCodeAdapter` beside Agnes and OpenAI. The adapter descriptor identifies its OpenAI-compatible outbound protocol and default Base URL. The generic relay handler continues streaming responses, while a request preparation hook can normalize the effective Base URL and optionally replace the request body before creating the upstream request.

The OpenCode public relay uses `/plugins/ai-relay/opencode/{slug}/v1/*`. The handler derives the endpoint path, loads the route, applies OpenCode URL normalization and optional chat-body conversion, then reuses the existing proxy client and response-copying behavior.

The plugin frontend continues loading platforms from `/api/platforms`. Platform descriptors expose `default_base_url`; selecting OpenCode in the create dialog fills `https://opencode.ai/zen` without affecting edit behavior.

## Error Handling

- Missing Bearer tokens, unknown routes, invalid Base URLs, proxy failures, and upstream failures use existing AI Relay errors.
- Request conversion is best-effort: malformed or unsupported payloads pass through unchanged rather than returning a conversion error.
- Existing Agnes and OpenAI route behavior remains unchanged.

## Testing

- Registry tests verify OpenCode metadata and ordering.
- URL tests verify automatic `/v1`, existing `/v1`, and relative mappings.
- Conversion tests cover model objects, system, parts, message fallback, preserved parameters, OpenAI pass-through, and invalid JSON pass-through.
- Handler tests verify the converted body, Bearer token, query string, and transparent upstream response.
- Frontend tests verify the OpenCode option and its default Base URL.
- Run all plugin-service Go tests, frontend unit tests, typecheck, and production build.

## Non-Goals

- No DeepSeek-specific retries or response logging.
- No Responses-to-Chat fallback or tool-call normalization.
- No Anthropic protocol support.
- No main-site frontend changes.
