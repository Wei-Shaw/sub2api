## 1. Backend Platform Model
- [x] 1.1 Add `xai` platform constants and supported account/platform checks.
- [x] 1.2 Add Grok text default model list and model mapping behavior.
- [x] 1.3 Ensure account create/update/list/test APIs accept and return `xai` accounts.
- [x] 1.4 Add xAI cloud model discovery via account-level `/models` refresh snapshots.

## 2. OAuth
- [x] 2.1 Implement xAI OAuth discovery, PKCE authorize URL, callback validation, token exchange, and credential persistence.
- [x] 2.2 Implement xAI token refresh using stored `refresh_token` and `token_endpoint`.
- [x] 2.3 Expose admin/OAuth endpoints for starting xAI login and submitting callback/code.
- [x] 2.4 Add tests for URL validation, authorize URL construction, callback parsing, token exchange metadata, and refresh metadata.

## 3. Text Gateway
- [x] 3.1 Route `xai` platform groups through a Grok text forwarding path for `/v1/responses`.
- [x] 3.2 Reuse or adapt existing Chat Completions to Responses conversion for `/v1/chat/completions`.
- [x] 3.3 Implement xAI request normalization for unsupported fields, tools, reasoning, and conversation hints.
- [x] 3.4 Forward to `<base_url>/responses` with xAI auth and accept headers for streaming/non-streaming.
- [x] 3.5 Preserve existing usage logging, account failover, concurrency, and billing behavior.
- [x] 3.6 Add tests for forwarding URL/header/body construction and stream/non-stream request paths.

## 4. Frontend
- [x] 4.1 Add `xai` platform labels/icons/options in account and group management.
- [x] 4.2 Add Grok OAuth entry in the admin OAuth flow.
- [x] 4.3 Add model whitelist/default model UI support for Grok text models.
- [x] 4.4 Add focused component/store tests where existing frontend tests cover platform options.
- [x] 4.5 Add an account test modal action to refresh xAI cloud models.

## 5. Verification
- [x] 5.1 Run targeted backend Go tests for account, OAuth, and gateway packages.
- [x] 5.2 Run targeted frontend typecheck/test commands.
- [ ] 5.3 If credentials are available, verify `/v1/models`, `/v1/responses`, and `/v1/chat/completions` against a real xAI account. (Not run: no real xAI credentials in this environment.)
- [x] 5.4 Verify the compatibility matrix: existing provider no-regression, Responses non-streaming, Responses streaming, Chat Completions conversion, model list ownership, model-aware reasoning, unsupported tool normalization, OAuth refresh, account failover, concurrency limits, usage logging, and billing.
- [x] 5.5 Verify cloud model parsing, account snapshot fallback, and `/v1/models` snapshot aggregation with targeted tests.
