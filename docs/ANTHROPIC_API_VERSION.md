# Claude API version

Anthropic accounts default to `anthropic-version: 2026-10-21`. The same version
policy applies to Messages, token counting, OAuth Claude Code mimicry, and the
Anthropic gateway's OpenAI-compatible request paths. Account connectivity tests,
upstream model discovery, and Anthropic channel monitoring also use this default.

Version selection, in priority order:

1. An enabled account-level `anthropic-version` header override (API Key accounts).
2. The client's explicit `anthropic-version` header, including in OAuth mimicry.
3. `2026-10-21` for Anthropic accounts.

This is a protocol version, not a model selection. It does not enable beta
features; those still require their corresponding `anthropic-beta` tokens.
Bedrock and Vertex retain their provider-specific version formats. Accounts on
other provider platforms retain their existing default.

## Migrating requests

API version `2026-10-21` rejects `temperature`, `top_p`, and `top_k`, even when
their values equal the old defaults. Remove those fields when using this version.
OAuth mimicry no longer inserts `temperature: 1` into new-version requests.

Messages forwarding preserves sampling parameters explicitly supplied by clients,
so invalid requests receive the upstream validation error instead of silently
losing their sampling settings. Token counting continues to remove sampling and
other generation-only parameters before forwarding. If your application needs
non-default sampling, explicitly send `anthropic-version: 2023-06-01` or configure
that version as an API Key account's header override. Legacy OAuth requests keep
the prior temperature top-up behavior.

The new version also rejects document blocks directly in assistant messages and
non-empty citations on user text blocks (including text inside search results).
Move documents to user messages and remove citations from user text. Assistant
text citations and document/search-result citation configuration remain valid.

See Anthropic's [version history](https://platform.claude.com/docs/en/api/versioning)
and [migration guide](https://platform.claude.com/docs/en/api/migration-2026-10-21)
for the complete protocol changes.
