## Why

OpenAI API-key accounts currently choose one preferred upstream text protocol. When an upstream supports both `/v1/chat/completions` and `/v1/responses`, that account-level choice forces one client protocol through a cross-protocol conversion. In a chained-gateway deployment this can alter prompt structure and cache behavior even though both gateways expose both native endpoints.

## What Changes

- Add a `preserve_inbound` value to `accounts.extra.openai_responses_mode` for OpenAI API-key accounts.
- Route inbound Chat Completions requests to upstream Chat Completions and inbound Responses requests to upstream Responses.
- Treat the mode as an explicit operator declaration that the upstream supports both endpoints.
- Keep the existing `chat_completions` endpoint capability gate and reject conflicting bulk updates that select embeddings-only capability.
- Add the mode to create, edit, and bulk-edit account forms with a dual-endpoint warning.

## Impact

- No database migration: the value uses the existing JSON `extra` field.
- Existing `auto`, `force_responses`, and `force_chat_completions` behavior is unchanged.
- Protocol preservation does not disable model mapping, billing, audit, safety filtering, or ordinary request normalization. Existing `openai_passthrough` remains the separate body-passthrough control.
