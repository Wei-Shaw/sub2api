## Context

The support-chat capability ships an SSE handler that forwards user conversations to an upstream LLM, and the support-knowledge-rag capability adds an embedding pipeline used by both FAQ CRUD (synchronous embed on save) and the document crawler (batch embed). Today both routes are credential-bound to the platform's **internal** `api_keys` table via the integer setting `support_chat_api_key_id`:

- The chat handler (`backend/internal/handler/support_chat_handler.go`) calls `apiKeyService.GetByID` to resolve the bearer token, then self-calls `http://127.0.0.1:<port>/v1/chat/completions`. This piggybacks on the platform's own gateway so usage is billed against the chosen internal key.
- The embedding service (`backend/internal/service/embedding_service.go`) uses the same indirection.

Operators upgrading from the chat-widget release have asked to point support-chat at an arbitrary OpenAI-compatible endpoint (OpenAI direct, DeepSeek, Azure OpenAI, internal vLLM/Ollama, third-party gateway) without going through the platform's internal billing. The proposal locks in the high-level direction (replace `support_chat_api_key_id` with `support_chat_llm_base_url` + `support_chat_llm_api_key`); this design doc nails down the technical shape: storage, validation, secret handling, dependency-injection ripple, and migration semantics.

## Goals / Non-Goals

**Goals:**

- Decouple the support widget's LLM transport from the platform's internal key/gateway plumbing — chat handler issues an outbound HTTPS request to an admin-configured `<base_url>/chat/completions` with a bearer token from a separate setting field.
- Share **one** credential pair between chat completions and embeddings (matches how OpenAI / Azure / DeepSeek / vLLM / Ollama-with-OpenAI-compat all expose `/chat/completions` and `/embeddings` under a single base URL with a single bearer token).
- Treat the api-key value as a secret: encrypt at rest using the platform's existing setting-secret pattern; never return the cleartext to the admin UI; PUT semantics support "leave unchanged" when the masked sentinel is sent back unmodified.
- Provide a "Test connection" admin action that issues a low-cost (`max_tokens=1`) probe so admins can verify the credential before saving — without persisting the test value if it fails.
- Cleanly remove the legacy `support_chat_api_key_id` field everywhere, with a single startup WARN for legacy installs (no automatic migration; the legacy key's actual bearer-token bytes may have rotated and self-call semantics differ from external semantics, so safe auto-migration is not possible).

**Non-Goals:**

- Allowing **separate** base_url/api_key for chat vs embedding (deferred — current proposal locks them to one pair under the OpenAI-compatible-upstream assumption). If a future provider splits them, we can add `support_chat_rag_embed_base_url` / `support_chat_rag_embed_api_key` overrides in a follow-up change.
- Supporting non-OpenAI-compatible APIs (Anthropic native, Google native, Cohere native). These will continue to require a translating gateway; no protocol-translation layer is in scope.
- Migrating historical `support_chat_api_key_id` values into the new fields automatically. The legacy value is an internal-key id, not a usable external bearer token; cross-walking it would require resolving the internal key's plaintext (which itself is encrypted) and assuming the upstream is the platform itself — a fragile and surprising outcome. We log a one-time WARN and require admin re-entry.
- Adding per-environment `.env` overrides for the credentials. They live in the settings table like every other admin-configurable value.

## Decisions

### Decision 1: Two new string settings, replace (not augment) `support_chat_api_key_id`

**Choice:** Add `support_chat_llm_base_url` (string ≤500 chars, must start with `http://` or `https://`) and `support_chat_llm_api_key` (string 1..500 chars, encrypted at rest), and **remove** `support_chat_api_key_id` from the codebase entirely.

**Alternative considered:** keep `support_chat_api_key_id` as a fallback — when the new fields are empty AND `api_key_id > 0`, fall back to the old self-call path.

**Why rejected:** keeping both code paths doubles the surface of the chat handler indefinitely, makes the spec ambiguous ("which path wins?"), and rewards admins who never migrate. The platform is pre-1.0 with a small operator base; a single-release breaking change with a clear startup WARN is preferable to a permanent dual-path.

### Decision 2: Share one `(base_url, api_key)` pair between chat and embedding

**Choice:** the embedding service and the chat handler read from the same two setting keys.

**Alternative considered:** four settings (`..._chat_base_url`, `..._chat_api_key`, `..._embed_base_url`, `..._embed_api_key`).

**Why rejected:** every realistic upstream (OpenAI, Azure OpenAI, DeepSeek, Together, Anyscale, vLLM, Ollama OpenAI-compat) exposes both `/chat/completions` and `/embeddings` on the same base URL with the same token. Doubling the setting surface buys nothing for the common case and clutters the admin UI. We retain the option to add embed-specific overrides later when a real splitting use case materializes (Non-Goals).

### Decision 3: Encrypt `support_chat_llm_api_key` at rest using the existing setting-secret pattern

**Choice:** use whatever encryption mechanism `support_chat_*` settings already use for secret-bearing fields (system-prompt, model). If the existing settings table stores plaintext for these fields, this change still leaves the field as a regular setting row but adds the convention in code: when reading via the admin response DTO, the api_key SHALL be replaced by a masked sentinel `"sk-***" + last4(value)` (or `"***"` if shorter than 4 chars). On UPDATE, when the request body equals the masked sentinel exactly, the service SHALL treat it as "leave unchanged" and skip the write for that field.

**Rationale:** matches how most admin UIs (GitHub Actions, GitLab CI, Cloud consoles) handle bearer tokens. Avoids accidental key leakage to the browser; survives "load → save without editing" without zeroing the secret.

**Implementation note for tasks.md:** verify whether the platform already encrypts setting values at rest (check `setting_service.go` for any AES/KMS path on writes). If not, this change does NOT introduce one — that's a cross-cutting concern out of scope. The masking-on-response and leave-unchanged-on-update semantics are sufficient for the immediate threat model (browser exposure / admin-page screenshot).

### Decision 4: Test-connection probe is a separate admin endpoint, not a side effect of save

**Choice:** add `POST /api/admin/support/chat/test-llm-connection` accepting `{base_url, api_key, model}` (admin can pass values from the form before persisting). Endpoint issues a 5-second-timeout `POST <base_url>/chat/completions` with body `{model, messages:[{role:"user",content:"ping"}], max_tokens:1, stream:false}` and returns `{ok: bool, status_code?: int, error?: string, latency_ms: int}`. The endpoint MUST NOT persist anything.

**Alternative considered:** validate-on-save by issuing the probe inside the settings PUT handler.

**Why rejected:** save-time validation couples admin save latency to upstream latency, complicates rollback ("save returned 502, did it persist anyway?"), and doesn't help the common pre-save "did I paste the right key?" UX moment. A standalone probe endpoint is also reusable for a future health-check dashboard.

### Decision 5: Strip `support_chat_api_key_id` from `SettingsResponse` and `SettingsUpdateRequest`, but read defensively at startup

**Choice:** the new code references `support_chat_api_key_id` in exactly **one** place: a startup migration hook that reads the raw setting row (bypassing the typed DTO), and emits a single `log.Warn` if the value is `> 0` AND the new `support_chat_llm_base_url` is empty. The setting row itself remains in the DB (we do not DELETE it — that risks data loss if rollback is needed); we just stop reading it through any typed path.

**Rationale:** clean removal from the type system (no field-name pollution in DTOs / wire-gen / public-settings filters) while still giving operators a clear log breadcrumb that they need to act.

### Decision 6: Frontend masks the api_key field with a "click to reveal / replace" pattern

**Choice:** the SettingsView form binds `support_chat_llm_api_key` to a text input of `type="password"` with a small "show" toggle button. On load, the input's value is the masked string from the API. When the admin types a new value, the form tracks an `apiKeyChanged` flag; `buildPayload` only includes the field when the flag is true (otherwise the backend sees the masked value and treats it as "leave unchanged" per Decision 3).

**Alternative considered:** always send the field, let the backend's leave-unchanged sentinel logic handle it.

**Why rejected:** sending the masked sentinel on every save creates noise in audit logs and risks a future regression where the sentinel string is mistaken for a legitimate bearer token. Frontend gating is cheap and explicit.

### Decision 7: Reuse the existing wire-gen graph after removing `apiKeyService` from `SupportChatHandler`

**Choice:** `SupportChatHandler` currently injects `apiKeyService` only for the `GetByID` call we're removing. After this change the handler should drop the dep. Run `go generate ./...` to regenerate `wire_gen.go`. The embedding service (which similarly drops the internal-key indirection) will likewise lose its `apiKeyService` dep if it was injected.

**Verification step in tasks.md:** confirm `apiKeyService` has no remaining references inside `support_chat_handler.go` and `embedding_service.go` before regenerating wire.

## Risks / Trade-offs

- **[Risk]** Operators upgrading don't read release notes, support widget silently breaks on next deploy.
  → **Mitigation:** startup WARN log when legacy `api_key_id > 0` AND new base_url empty; admin Settings page shows a yellow banner above the Support Chat section when LLM is enabled but base_url+api_key are not both set; chat handler returns `503 config_error` (already implemented for the api_key_id missing case, just retargeted to the new fields).

- **[Risk]** Admin pastes a bearer token into the base_url field by accident (or vice versa). The probe-endpoint catches obvious mismatches but not all (e.g. `https://sk-abc123` is technically a parseable URL).
  → **Mitigation:** validation rule `support_chat_llm_api_key` MUST start with one of `[sk-, Bearer , api-, key-]` OR be at least 20 chars (heuristic); `support_chat_llm_base_url` MUST end with `/v1` OR contain a path segment longer than `/` (heuristic to catch bare-token-pasted-as-URL). Heuristics are non-blocking — they generate a warning in the response, not an error — to avoid rejecting unconventional but valid configs (custom gateways with `/api` prefixes, etc).

- **[Risk]** Bearer-token rotation: when the admin rotates the upstream key, they must re-save the setting. If they rotate without re-save, all chats start failing 401.
  → **Mitigation:** the chat handler surfaces upstream 401 as `event: error\ndata: {"error":{"message":"Upstream authentication failed — verify support_chat_llm_api_key in admin settings", "type":"upstream_auth"}}\n\n`. No automatic recovery (this is a human-in-the-loop concern).

- **[Risk]** Encrypted-at-rest is not a strict project pattern; we may inadvertently store plaintext in the settings table.
  → **Mitigation:** the threat model treats DB access as already-trusted (the platform's own DB user can read all keys). The masked-on-response + leave-unchanged-on-update protections defend against the realistic threats (browser DevTools / over-the-shoulder / accidental log dump). If a stronger threat model emerges later (multi-tenant DB, untrusted ops staff), a follow-up change can introduce KMS-backed encryption for settings rows tagged `secret`.

- **[Trade-off]** Removing `support_chat_api_key_id` is a hard breaking change. We accept this because the chat-widget feature shipped less than 24 hours ago (per the recent archive timestamps) and has minimal production exposure; a clean break is preferable to permanent legacy support.

## Migration Plan

1. **Code rollout (single PR):** ship all backend + frontend changes together. The new typed DTO removes `support_chat_api_key_id`; the legacy setting row remains in DB until an admin clears it via the admin UI's "reset to default" affordance (out of scope here).
2. **Startup hook:** on application boot, the support-chat domain service reads the raw `support_chat_api_key_id` row via the generic settings repository; if present and non-zero AND the new `support_chat_llm_base_url` is empty, log `WARN: legacy support_chat_api_key_id detected; please reconfigure with support_chat_llm_base_url + support_chat_llm_api_key in the admin Settings page (Support Chat section). LLM chat will be disabled until reconfigured.` exactly once.
3. **Admin notice:** the SettingsView surfaces a one-time banner explaining the change, with a "Got it" dismiss that writes a new public_setting `support_chat_legacy_credential_warning_dismissed=true` (or simply leaves it permanently visible until base_url+api_key are configured — TBD in tasks.md, simpler is better).
4. **Rollback:** if the change must be reverted, redeploy the previous binary; the old code reads `support_chat_api_key_id` from DB unchanged (we never DELETE the row), so service resumes immediately. The new fields would be ignored by the old binary — no DB cleanup needed.
