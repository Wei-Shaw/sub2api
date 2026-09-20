# Automatic API key protection

Opt-in, reversible substitution for recognizable API keys in user-supplied text. Users paste credentials normally; the gateway sends placeholders upstream and restores matching placeholders in client reply text and tool arguments. The feature is disabled by default.

## Request-local design

1. Authenticate and apply request limits. Load the policy and select the authenticated user/group.
2. Parse message content and tool arguments, replace recognized credentials, and retain a **request-local** placeholder-to-credential map. Replace every request-body reread hook before snapshots, external moderation, logging, retries and provider conversion.
3. Forward the protected content. Model-facing history and regular diagnostics contain placeholders.
4. After provider conversion, restore only complete placeholders present in this request's map, in reply text and tool arguments. JSON and SSE are decoded by protocol; parallel choices and tool calls have separate buffers.

A placeholder is `keyx_` followed by the lowercase SHA256 hex digest of the credential (64 characters). It is deterministic across requests, users, instances and restarts. This preserves stable substituted prompt content without a new platform secret or per-service registration. The digest is not encryption: matching credentials are linkable and guessable credentials can be tested offline. Rules should target recognizable high-entropy service keys, not ordinary passwords.

SHA256 cannot recover a credential. Restoration uses only the map built from the **current request**, never a global dictionary, Redis lookup, client-supplied identity or response ID. Different users may receive the same placeholder for the same credential; knowing that placeholder does not grant access to another request's map. Unknown, modified, truncated or extended placeholders remain unchanged.

The map is released with the request; no credential mappings are persisted to Redis, a database or files. A full-history client resends the original prompt and restored tool results each round, so the gateway scans them again and rebuilds the map. A request containing only an old placeholder cannot recover its original value, even from the same user/session.

## Configuration

Admin UI: **System Settings → Security & Authentication → Automatic key protection**. Keep the master switch off for existing behavior. Select users/groups, or leave both lists empty to apply to everyone when enabled. A match in either list enables protection.

The existing settings store holds this non-secret policy; no schema migration is required. Admin API: `GET` and `PUT /api/v1/admin/settings/key-protection`.

```json
{
  "enabled": false,
  "user_ids": [],
  "group_ids": [],
  "rules": [],
  "custom_rules": []
}
```

- `rules`: an empty list enables all built-in rules. A nonempty list is a whitelist; omit a rule to disable it. Built-ins: `openai`, `anthropic`, `github`, `gitlab`, `google`, `stripe`, `slack`, `huggingface`, `groq`, `npm`, `private_key`.
- `custom_rules`: editable/deletable Go/RE2 regex rules. Example: `[{"name":"demo","pattern":"demo_[A-Za-z0-9]{32}"}]`. The complete match is replaced. Names must be unique, 1–48 lowercase letters/digits/underscores, starting with a letter. Maximum 32 custom rules; patterns are limited to 2,048 bytes and must not match empty strings. Backend validation rejects invalid rules without logging their text.
- Both ordinary reply text and tool arguments are restored. There are no masking-only, tools-only, retention, storage or mapping-cleanup controls in this version.
- Fixed limits: 256 distinct credentials, 1 MiB per credential and 8 MiB of mapping data per request; existing body limits also apply. Exceeding a limit fails before forwarding.

The policy is snapshotted per request. Disabled/unselected requests retain their original body processing. The current implementation still reads settings on each request; an unavailable or invalid policy returns a protection error rather than silently disabling an enabled policy.

## SSH and PEM private keys

The default `private_key` rule replaces complete `OPENSSH PRIVATE KEY`, `RSA PRIVATE KEY`, `EC PRIVATE KEY`, `DSA PRIVATE KEY`, PKCS#8 `PRIVATE KEY` and `ENCRYPTED PRIVATE KEY` blocks with one placeholder. It preserves the BEGIN/END labels, base64 text, LF/CRLF inside the block, and legacy encrypted PEM headers such as `Proc-Type` / `DEK-Info`. Whitespace outside the block remains ordinary context; normal key-file writing should retain a final newline.

No key is decrypted, validated against a server or loaded from the filesystem. Public keys, fingerprints and certificates are unchanged. Known private-key headers with truncated, mismatched, malformed or unsafe overlapping content fail closed. PuTTY `.ppk`, PGP private-key blocks and other recognized `-----BEGIN ... PRIVATE KEY-----` labels are explicitly rejected when this rule applies; unlabelled binary/DER/base64 data is not detected by this rule. To opt in from an existing nonempty whitelist, add `private_key`; an empty whitelist already includes it.

Format references: [OpenSSH private-key format](https://github.com/openssh/openssh-portable/blob/master/PROTOCOL.key), [PKCS#8 textual encodings](https://www.rfc-editor.org/rfc/rfc7468.html#section-10). Encrypting a PEM file does not change its treatment as user-supplied text; opaque encrypted protocol-history fields remain unsupported.

## Supported paths and limitations

| Path | JSON | SSE | Notes |
| --- | --- | --- | --- |
| `/v1/chat/completions`, `/chat/completions` | Yes | Yes | Text and function arguments |
| `/v1/responses`, `/responses`, `/backend-api/codex/responses` | Yes | Yes | Full-history input only |
| `/v1/messages`, `/antigravity/v1/messages` | Yes | Yes | Text and tool input/result content |
| Gemini, WebSocket, compact/background, image/audio/video/embedding generation, token-count paths | No | No | Protected requests are rejected |

`previous_response_id`, Responses `conversation`, encrypted/signed history and signed request bodies cannot safely use a request-local map and are rejected when protection applies. Send full history instead. Protected HTTP requests remain on verified HTTP forwarding paths. Model/usage discovery reads remain available.

Text scanning does not inspect image/audio bytes or promise to recognize arbitrary passwords. Unknown or unsafe content structures that cannot be transformed safely fail explicitly. Tool names, model IDs, authentication headers and platform provider credentials are not substitution targets.

Successful scanning means no *supported-format* key was found or that supported matches were replaced; it does not certify absence of all sensitive information. Input failures never fall back to forwarding the original body. Stream failures discard unchecked tails and terminate with the protocol error. Tool arguments may be buffered until complete; normal text uses a bounded placeholder tail.

The gateway adds `Cache-Control: no-store, private` to all protected client responses, including requests without a match. It bypasses legacy reasoning/compatibility continuation state that lacks the required context. Deterministic hashes stabilize upstream prompt content; this feature does **not** promise every pre-existing cache path is unchanged. Usage and billing continue to use upstream-reported data.

Normal logs contain rule counts, timing and failure categories, not credentials or restored response bodies. Protected client error bodies are excluded from the ordinary operations capture. Once restored, credentials are available to the client and tool executor. This feature does not restrict credential permissions or validate where model-generated commands send them.

## Fictional round-trip example

Fixture (not a real credential): `ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA`.

```text
Client input:  Use ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA to query the repository.
Upstream:      Use keyx_49e7237e11464693589bca95fe317f2fde5793bb7d70da9749f55641a5fff406 to query the repository.
Model output:  {"api_key":"keyx_49e7237e11464693589bca95fe317f2fde5793bb7d70da9749f55641a5fff406"}
Client result: {"api_key":"ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
```

Repeating that input produces the same placeholder. Sending only the placeholder in a fresh request leaves it unchanged. The executable Go example and gateway tests assert this round trip with fictional credentials and an in-memory mock upstream.

## Validation

From `backend`, with the version required by `go.mod`:

```sh
go test ./internal/keyprotection
go test -tags=unit ./internal/server/middleware ./internal/server/routes ./internal/handler ./internal/handler/admin ./internal/service ./internal/securityaudit ./internal/pkg/apicompat
go vet ./internal/keyprotection ./internal/server/middleware ./internal/server/routes ./internal/handler ./internal/handler/admin ./internal/service
```

Tests cover SHA256 stability, duplicate/multiple keys, request/user isolation, full-history rebuilding, unsupported continuation, forged placeholders, stream split positions, JSON escaping, parallel tools, cancellation, retries, default-off behavior, rule validation, false-positive fixtures, and protected audit/log paths. Frontend checks cover the simplified settings API and component.

Local acceptance additionally runs the built gateway with PostgreSQL/Redis and an actual OpenCode client against a deterministic local model mock. Redis remains a normal Sub2API dependency, not a credential-map store. No real provider key or paid model is used. Acceptance scripts and credential-bearing local artifacts are excluded from this feature branch.

Verified locally on 2026-09-13: 24 gateway acceptance checks (including 204 SSE split cases), four audit checks, three actual OpenCode protocol runs with tool execution and full-history continuation, and eight browser checks passed. Related Go package suites, vet, frontend tests/type checks and the embedded application build also passed.

SSH/PEM extension verified on 2026-09-14: core/middleware suites and existing SSE split/tool matrices with LF/CRLF private-key fixtures passed, as did 21 live JSON format/protocol round trips and five unsafe-input rejections. Actual OpenCode passed all three protocols using a newly generated, never-authorized local Ed25519 fixture; restored files matched the original bytes and derived the same public key via `ssh-keygen` after applying private-file permissions. No SSH server connection was made. Frontend build/type/i18n checks and Go vet/build passed.
