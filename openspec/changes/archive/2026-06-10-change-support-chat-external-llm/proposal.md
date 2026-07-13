## Why

The current support-chat capability binds the floating customer-service widget (and its RAG embedding pipeline) to the platform's **own** LLM gateway: admins must pick an existing internal `api_keys` row via `support_chat_api_key_id`, and the chat handler self-calls `http://127.0.0.1:<port>/v1/chat/completions`. This forces every support-chat token through the platform's billing path, prevents operators from pointing the widget at a cheaper external provider (OpenAI direct, Azure OpenAI, third-party gateway, self-hosted vLLM/Ollama), and couples the widget's availability to the platform's upstream provider configuration. The same lock-in applies to RAG embeddings, which currently reuse the same internal-key indirection.

Operators have asked to plug the widget into an arbitrary OpenAI-compatible endpoint, with credentials configured directly in the admin settings (the same way one would configure any third-party API integration), so that support-chat traffic and embedding traffic are isolated from the platform's main billing surface and unaffected by changes to the platform's upstream provider list.

## What Changes

- **BREAKING**: replace the integer setting `support_chat_api_key_id` with two string settings:
  - `support_chat_llm_base_url` — OpenAI-compatible HTTP base URL (e.g. `https://api.openai.com/v1`, `https://api.deepseek.com/v1`, internal gateway URL).
  - `support_chat_llm_api_key` — bearer token, stored encrypted at rest and write-only in the admin API (response masks to `sk-***last4`).
- Chat handler (`POST /api/v1/support/chat`) SHALL forward to `<base_url>/chat/completions` with `Authorization: Bearer <api_key>` instead of self-calling `127.0.0.1:<port>/v1/chat/completions`. Self-calling logic and the `apiKeyService.GetByID` lookup are removed from the handler.
- RAG embedding service (FAQ create/update, FAQ reindex, doc pipeline embed batches, chat handler query embedding) SHALL use the same `<base_url>/embeddings` endpoint with the same `<api_key>` (single shared credential pair for chat + embedding, matching how OpenAI / Azure / DeepSeek / etc. expose both endpoints under one base URL).
- Admin settings UI: replace the "API Key 选择" dropdown with two text inputs (URL + masked password). Add a "Test connection" button that issues a tiny `POST <base_url>/chat/completions` with `model=<support_chat_model>` and 1 token to verify credentials before save.
- Seed migration: on application startup, when the legacy `support_chat_api_key_id > 0` exists and the new `support_chat_llm_base_url` is empty, the system SHALL log a one-time WARN advising the admin to reconfigure with explicit base_url + api_key (no automatic migration — the legacy key's actual `key` value cannot be safely re-derived as a standalone bearer token because internal keys may have been rotated, and self-calling semantics differ from external semantics).
- Validation: `support_chat_llm_base_url` MUST be a valid `http(s)://` URL ≤500 chars; `support_chat_llm_api_key` MUST be 1..500 chars; when `support_chat_llm_enabled = true`, both MUST be non-empty.
- Public settings payload SHALL NOT expose either new field (admin-internal). The existing public field `support_chat_enabled` is unaffected.
- The legacy `support_chat_api_key_id` setting key SHALL be **removed** from the codebase (DTO, service, handler, frontend, i18n). Defensive read in startup migration only — purely to detect and warn legacy installs.

## Capabilities

### New Capabilities

_(none — this change only modifies existing capabilities)_

### Modified Capabilities

- `support-chat`: replace the `support_chat_api_key_id` reference in the "SSE Chat Endpoint with Turn and Token Caps" requirement and the "Public Settings Surface for Chat Widget" requirement with the new `support_chat_llm_base_url` + `support_chat_llm_api_key` pair; update the forwarding semantics (external endpoint instead of self-call). Remove the api_key_id existence-validation clause.
- `support-knowledge-rag`: update embedding-service references throughout the "FAQ Item CRUD" / "Document Pipeline" / "Vector Retrieval" requirements to read credentials from the new shared settings pair instead of resolving an internal api_key_id; explicitly note that chat and embedding share a single `(base_url, api_key)` pair under the assumption of an OpenAI-compatible upstream that exposes both `/chat/completions` and `/embeddings`.

## Impact

**Affected code**

- Backend setting layer: `backend/internal/service/domain_constants.go` (key constants + defaults), `backend/internal/service/setting_service.go` (DTO field, validation, seed defaults, runtime view, public-settings filter), `backend/internal/handler/dto/settings.go` (`SettingsResponse` + `SettingsUpdateRequest`), `backend/internal/handler/admin/setting_handler.go` (api_key_id existence/admin-owned validation removal — currently a TODO comment).
- Backend chat handler: `backend/internal/handler/support_chat_handler.go` (replace `selfCallURL` + `apiKeyService.GetByID` with direct `http.NewRequest` to `<base_url>/chat/completions`; drop the api_key_service dependency injection if no other consumer remains).
- Backend embedding service: `backend/internal/service/embedding_service.go` (replace internal-key lookup with `(base_url, api_key)` reading; same call site for chat-handler RAG embedding, FAQ CRUD synchronous embedding, doc pipeline batch embedding, FAQ reindex).
- Wire-gen impact: removing `apiKeyService` dependency from `SupportChatHandler` may simplify the constructor; rerun `go generate ./...` for `wire_gen.go`.
- Frontend admin settings: `frontend/src/api/admin/settings.ts` (DTO mirror), `frontend/src/views/admin/SettingsView.vue` (replace ApiKey dropdown widget with two text inputs + masked password rendering + test-connection button + form initializer + `buildPayload` serialization), `frontend/src/i18n/locales/{zh,en}.ts` (label/help/error keys for the new fields).
- DB migration: no schema change (settings are key-value strings); only data migration via warning-on-startup. The encrypted-at-rest concern is handled by the existing setting encryption pattern (if any) — see design.md for whether a new encrypted column is required or the value can be stored as-is alongside the existing `value` column.

**Breaking change scope**

- Operators upgrading from the previous release MUST manually re-enter `support_chat_llm_base_url` and `support_chat_llm_api_key` for the support widget. Without this, `support_chat_llm_enabled = true` will fail validation and the chat input will fall back to the disabled-state notice. Document this prominently in the upgrade notes.
- Any external code reading `support_chat_api_key_id` (none expected — the field is admin-only) will need to migrate.
