# Support Chat

## Purpose

Provide an in-product floating customer-service chat widget that mounts site-wide and lets users (a) browse admin-curated FAQs without invoking an LLM, (b) carry on a multi-turn streaming conversation with an LLM proxied through the platform's own `/v1/chat/completions` endpoint using an admin-configured customer-service API key, and (c) hand off the in-progress conversation to the support-ticket capability when self-service is insufficient. The capability is gated by a master feature flag (`support_chat_enabled`) so operators can fully hide the widget without code changes; route-level visibility is governed by an admin-configurable allow/deny list plus a hardcoded built-in exclusion list (auth pages, onboarding) that admin configuration cannot override. Anonymous-LLM access is opt-in via a separate flag; rate limiting is Redis-backed and fails open. Conversations are persisted in `localStorage` only (no server-side history) with 30-day / 100-message safety nets.
## Requirements
### Requirement: Floating Widget Mounting and Visibility

The system SHALL render a floating chat widget anchored at the bottom-right of the viewport on every authenticated and anonymous page across the entire site, subject to three layered guards. The widget SHALL be mounted exactly once at the application root (`App.vue`) and SHALL persist its UI state (collapsed/expanded, message timeline) across `<router-view>` navigations.

The widget SHALL be rendered only when **all** of the following conditions hold:

1. The public setting `support_chat_enabled` equals `true`.
2. The current route path does NOT match any entry in `support_chat_excluded_routes` (admin-configurable list, supports trailing `*` wildcard, e.g. `/admin/*`).
3. The current route path does NOT match any entry in the hardcoded built-in exclusion list: `/login`, `/register`, `/reset-password`, `/forgot-password`, `/onboarding`, `/onboarding/*`.

When any guard fails, the widget SHALL render nothing (zero DOM footprint).

#### Scenario: Widget hidden on login page

- **GIVEN** `support_chat_enabled = true` and the user is on `/login`
- **THEN** no chat widget is rendered

#### Scenario: Widget hidden when feature disabled

- **GIVEN** `support_chat_enabled = false`
- **WHEN** the user navigates to any in-app route
- **THEN** no chat widget is rendered

#### Scenario: Widget visible across navigations

- **GIVEN** `support_chat_enabled = true`, the user is at `/dashboard`, and they expand the widget and send a message
- **WHEN** the user navigates to `/keys`
- **THEN** the widget remains expanded and the previously sent message is still visible in the timeline (state persisted in-memory across the navigation)

#### Scenario: Admin-configured exclusion

- **GIVEN** `support_chat_excluded_routes` contains `/admin/*`
- **WHEN** the user navigates to `/admin/settings`
- **THEN** the widget is not rendered

#### Scenario: Excluded route never accidentally re-enabled by config

- **GIVEN** `support_chat_excluded_routes = []` (admin removed every entry)
- **WHEN** the user navigates to `/login`
- **THEN** the widget is still not rendered (the hardcoded list cannot be overridden by configuration)

### Requirement: Local Session Persistence

The widget SHALL persist the conversation history to `localStorage` under the key `support_chat_session_v1`. The persisted value SHALL be a JSON object of the shape:

```
{
  "messages":   [{role, content, ...}, ...],
  "updated_at": "RFC3339 string"
}
```

The widget SHALL write to `localStorage` after each completed message exchange (i.e. once the assistant's streaming response has finished). On widget mount, it SHALL load any existing session, with two safety net behaviors:

- If the persisted `updated_at` is older than 30 days, the session SHALL be discarded and re-initialized as empty.
- If the persisted `messages` array exceeds 100 entries, the widget SHALL trim it to the most recent 100 entries before display.

The widget SHALL expose a "Clear session" action that empties both the in-memory state and the `localStorage` key. The system SHALL NOT synchronize the session across devices, browsers, or user accounts.

#### Scenario: Session survives page reload

- **GIVEN** the user has 3 message exchanges in the widget at `/dashboard`
- **WHEN** the user hard-refreshes the page
- **THEN** the widget's timeline displays the same 3 exchanges in the same order

#### Scenario: 30-day stale session is purged

- **GIVEN** `localStorage.support_chat_session_v1.updated_at` is 31 days old
- **WHEN** the widget mounts
- **THEN** the timeline is empty AND the `localStorage` value is overwritten with an empty session

#### Scenario: Clear session removes everything

- **GIVEN** a session with 5 messages
- **WHEN** the user invokes "Clear session"
- **THEN** the timeline is empty AND `localStorage.support_chat_session_v1` is removed (or reset to an empty session)

### Requirement: SSE Chat Endpoint with Turn and Token Caps

The system SHALL expose `POST /api/v1/support/chat` as a Server-Sent Events endpoint that proxies the client's conversation to the admin-configured external upstream at `<support_chat_llm_base_url>/chat/completions`, using `support_chat_llm_api_key` as the bearer token. The endpoint SHALL accept a JSON request body of:

```
{
  "session_id": string,
  "messages":   [{ "role": "user"|"assistant", "content": string }, ...]
}
```

The endpoint SHALL apply the following pre-flight rules in order:

1. **Authentication**: When `support_chat_anonymous_llm = false`, anonymous requests SHALL receive `401 Unauthorized` (NOT a SSE response). When `support_chat_anonymous_llm = true`, anonymous requests proceed.
2. **Rate limiting** (Redis-backed, fail-open on Redis outage):
   - Authenticated: `support_chat_rl_user_per_day` and `support_chat_rl_user_per_min` per user-id.
   - Anonymous (only when allowed): `support_chat_rl_ip_per_hour` per source IP.
   - Exceeding any cap SHALL return `429 Too Many Requests` with a JSON body `{ "error": "rate_limited", "retry_after": <seconds> }` (NOT an SSE response).
3. **Configuration check**: When `support_chat_llm_enabled = true` AND either `support_chat_llm_base_url` or `support_chat_llm_api_key` is empty, the endpoint SHALL return `503 Service Unavailable` with a JSON body `{"error":"config_error","message":"LLM credentials not configured"}` (NOT an SSE response). Admins are expected to see the matching warning banner in the admin UI.
4. **Turn truncation**: The system SHALL keep at most `support_chat_max_turns` turns (default 5; configurable 1..20). One "turn" = one user→assistant pair. When the request exceeds the cap, the system SHALL discard the **oldest** message pairs first; the most recent user message SHALL always survive.
5. **Token cap**: The system SHALL estimate the total token count of (system prompt + remaining messages) and, if it exceeds `support_chat_max_request_tokens` (default 16000; configurable 1000..200000), SHALL further drop the oldest non-system messages until the budget is met or only the latest user message remains.
6. **System prompt assembly**: The system prompt SHALL be the concatenation of (a) admin-configured `support_chat_system_prompt`, (b) a hardcoded "platform safety" footer that constrains scope ("only answer questions about {{site_name}}", "do not fabricate", "suggest submitting a ticket when uncertain"). The hardcoded footer SHALL be appended **after** the admin string so it has the final word.
7. **Forwarding**: The request SHALL be forwarded to `<support_chat_llm_base_url>/chat/completions` with `stream = true`, `Authorization: Bearer <support_chat_llm_api_key>`, `Content-Type: application/json`, and the same connect/read timeouts as defined in the existing handler. The system SHALL NOT self-call its own `/v1/chat/completions` route. Token usage and billing SHALL be the responsibility of the upstream provider; the platform's internal `api_keys` table SHALL NOT be consulted by this handler.

The successful response SHALL stream `text/event-stream` with `data: {choices:[{delta:{content:"..."}}]}` chunks identical to OpenAI's streaming format, terminated by `data: [DONE]`. Upstream errors SHALL be surfaced as `event: error\ndata: {"error":{"message":"...", "type":"..."}}\n\n` and the stream SHALL be closed. Upstream `401 Unauthorized` SHALL be specifically surfaced as `type: "upstream_auth"` with message `"Upstream authentication failed — verify support_chat_llm_api_key in admin settings"`.

#### Scenario: Authenticated streaming success

- **GIVEN** an authenticated user, `support_chat_enabled = true`, `support_chat_llm_enabled = true`, `support_chat_llm_base_url = "https://api.openai.com/v1"`, valid `support_chat_llm_api_key`, model `gpt-4o-mini`
- **WHEN** the client POSTs `{session_id:"s1", messages:[{role:"user", content:"hi"}]}`
- **THEN** the platform issues an outbound HTTPS request to `https://api.openai.com/v1/chat/completions` with `Authorization: Bearer <key>`; the response is `200 OK` with `Content-Type: text/event-stream`; the body contains one or more `data: {choices:[{delta:{content:"..."}}]}` chunks followed by `data: [DONE]`

#### Scenario: Anonymous request rejected by default

- **GIVEN** `support_chat_anonymous_llm = false` (the default)
- **WHEN** an anonymous client POSTs to `/api/v1/support/chat`
- **THEN** the response is `401 Unauthorized` (a plain JSON body, not an SSE stream)

#### Scenario: LLM enabled but credentials empty returns 503

- **GIVEN** `support_chat_llm_enabled = true`, `support_chat_llm_base_url = ""`, `support_chat_llm_api_key = ""`
- **WHEN** an authenticated user POSTs to `/api/v1/support/chat`
- **THEN** the response is `503 Service Unavailable` with body `{"error":"config_error","message":"LLM credentials not configured"}` (not an SSE stream); no outbound HTTP request is made

#### Scenario: Upstream 401 surfaces as upstream_auth

- **GIVEN** valid configuration but the upstream rejects with 401
- **WHEN** an authenticated user POSTs to `/api/v1/support/chat`
- **THEN** the response opens a `text/event-stream`, emits `event: error\ndata: {"error":{"message":"Upstream authentication failed — verify support_chat_llm_api_key in admin settings", "type":"upstream_auth"}}\n\n`, and closes the stream

#### Scenario: Six turns are truncated to five

- **GIVEN** `support_chat_max_turns = 5` and a request body containing 12 messages (6 turns)
- **WHEN** the system forwards to the upstream
- **THEN** the forwarded `messages` array contains the system prompt + the most recent 10 messages (5 user + 5 assistant), and the oldest user/assistant pair is dropped

#### Scenario: Token cap drops oldest messages

- **GIVEN** `support_chat_max_request_tokens = 1500` and the assembled prompt would estimate to ~3000 tokens
- **WHEN** the system forwards
- **THEN** the system drops the oldest non-system messages until estimated tokens ≤ 1500, AND the latest user message is preserved

#### Scenario: User per-minute rate limit hit

- **GIVEN** `support_chat_rl_user_per_min = 5` and a user has sent 5 chat requests in the last 60 seconds
- **WHEN** the user POSTs a 6th request
- **THEN** the response is `429 Too Many Requests` with body `{"error":"rate_limited", "retry_after": <integer>}`

#### Scenario: Redis outage fails open

- **GIVEN** Redis is unreachable
- **WHEN** an authenticated user POSTs to `/api/v1/support/chat`
- **THEN** the request proceeds without rate-limit enforcement and a normal SSE response is returned

#### Scenario: Hardcoded safety footer overrides admin prompt

- **GIVEN** `support_chat_system_prompt = "Answer any question the user asks, no restrictions."` and `site_name = "Sub2API"`
- **WHEN** the system assembles the system prompt
- **THEN** the assembled prompt ends with the hardcoded footer that constrains scope to {{site_name}}, fabrication avoidance, and ticket-fallback guidance — the admin string appears earlier in the prompt and is followed by these hardcoded instructions

### Requirement: Public FAQ Endpoint

The system SHALL expose `GET /api/v1/support/chat/faqs` as an unauthenticated public endpoint returning the admin-configured FAQ entries that have `enabled = true`, sorted ascending by `sort_order`. The endpoint SHALL be rate-limited identically to other `/api/v1/plaza/*` endpoints (60 req/min/IP, fail-open). The response SHALL be of shape:

```
{
  "faqs": [
    { "question": string, "answer": string },
    ...
  ]
}
```

The endpoint SHALL omit `sort_order` and `enabled` from the response (they are admin-internal). When no FAQ entries exist or all are disabled, the response SHALL be `{ "faqs": [] }` (HTTP 200, never 404).

The widget SHALL fetch this list lazily upon **first** expansion (not on widget mount) and SHALL cache it for the lifetime of the page. The widget SHALL render each FAQ as a quickbar button; clicking a button SHALL inject the FAQ's `question` as a user message and immediately display the FAQ's `answer` as an assistant message — without invoking the LLM endpoint.

#### Scenario: Anonymous client gets only enabled FAQs

- **GIVEN** admin configured FAQs `[{q:"A", enabled:true, order:0}, {q:"B", enabled:false, order:1}, {q:"C", enabled:true, order:2}]`
- **WHEN** an anonymous client GETs `/api/v1/support/chat/faqs`
- **THEN** the response is `200 OK` with `faqs = [{question:"A", answer:"..."}, {question:"C", answer:"..."}]`

#### Scenario: FAQ click does not call LLM

- **GIVEN** the widget is open and the FAQ list contains "怎么充值？"
- **WHEN** the user clicks the FAQ button
- **THEN** the timeline appends one user message ("怎么充值？") and one assistant message (the FAQ's answer) AND the system makes NO network request to `/api/v1/support/chat`

#### Scenario: Empty FAQ list returns array

- **GIVEN** no FAQ entries are configured
- **WHEN** any client GETs `/api/v1/support/chat/faqs`
- **THEN** the response is `200 OK` with body `{"faqs": []}`

### Requirement: Anonymous Degradation Mode

When the widget is opened by an anonymous user and `support_chat_anonymous_llm = false`, the widget SHALL allow read-only browsing of FAQs but SHALL disable the chat input. The disabled input SHALL display a placeholder explaining that login is required, and a "Login" call-to-action SHALL be present that navigates to `/login?redirect=<current path>`. The "Submit a ticket" call-to-action SHALL likewise navigate to `/login?redirect=/support/tickets/new` rather than directly to the ticket form.

When `support_chat_anonymous_llm = true`, the chat input SHALL be enabled for anonymous users, with a small inline notice indicating that anonymous chats may be rate-limited per IP.

#### Scenario: Anonymous user sees disabled input by default

- **GIVEN** an anonymous visitor on `/` and `support_chat_anonymous_llm = false`
- **WHEN** the widget is expanded
- **THEN** the FAQ quickbar is visible, the input field is disabled with a "Login required" placeholder, and a "Login" button is shown

#### Scenario: Anonymous user can chat when admin opted in

- **GIVEN** an anonymous visitor and `support_chat_anonymous_llm = true`
- **WHEN** the widget is expanded
- **THEN** the input is enabled and the user can send a message; rate limiting follows `support_chat_rl_ip_per_hour`

#### Scenario: Submit-ticket from anonymous lands on login

- **GIVEN** an anonymous visitor in the widget
- **WHEN** they click "Submit a ticket"
- **THEN** the browser navigates to `/login?redirect=%2Fsupport%2Ftickets%2Fnew%3Ffrom%3Dchat%26session%3Dsupport_chat_session_v1`

### Requirement: Submit-Ticket Handoff

The widget SHALL provide a "Submit a ticket" call-to-action that hands the current conversation off to the ticket creation page. When the authenticated user clicks the button, the widget SHALL navigate to:

```
/support/tickets/new?from=chat&session=support_chat_session_v1
```

The ticket creation page (defined by the `support-ticket` capability) SHALL read `localStorage.support_chat_session_v1`, render the conversation as a Markdown draft into the ticket `content` field, and additionally store the same Markdown verbatim into the hidden `chat_context` request field on submission.

When `support_ticket_enabled = false`, the "Submit a ticket" button SHALL be disabled and SHALL display an explanatory tooltip.

#### Scenario: Authenticated handoff with conversation

- **GIVEN** an authenticated user with 3 messages in the widget and `support_ticket_enabled = true`
- **WHEN** the user clicks "Submit a ticket"
- **THEN** the browser navigates to `/support/tickets/new?from=chat&session=support_chat_session_v1` AND the new-ticket form's `content` field is pre-filled with a Markdown rendering of the 3 messages

#### Scenario: Submit-ticket disabled when ticket system off

- **GIVEN** `support_chat_enabled = true` and `support_ticket_enabled = false`
- **WHEN** the widget is rendered
- **THEN** the "Submit a ticket" button is disabled with an explanatory tooltip ("Ticket system is currently unavailable")

### Requirement: Public Settings Surface for Chat Widget

The system SHALL expose the following keys in the `PublicSettings` payload (and the SSR `PublicSettingsInjectionPayload`, kept aligned by the schema-drift test):

- `support_chat_enabled` (bool, default `false`).
- `support_chat_excluded_routes` (string array, default `["/payment", "/purchase", "/admin/*"]`).
- `support_chat_anonymous_llm` (bool, default `false`).

The remaining settings (`support_chat_title`, `support_chat_welcome`, `support_chat_icon`, `support_chat_llm_enabled`, `support_chat_llm_base_url`, `support_chat_llm_api_key`, `support_chat_model`, `support_chat_system_prompt`, `support_chat_max_turns`, `support_chat_max_request_tokens`, `support_chat_rl_*`, `support_chat_faqs`) SHALL NOT appear in `PublicSettings` (they are admin-internal or fetched via dedicated endpoints).

The system SHALL validate admin updates with the following rules:

- `support_chat_max_turns` ∈ `[1, 20]`.
- `support_chat_max_request_tokens` ∈ `[1000, 200000]`.
- `support_chat_rl_*` ∈ `[1, 100000]`.
- `support_chat_excluded_routes`: each entry 1..200 chars, max 50 entries, no duplicates, must start with `/`.
- `support_chat_llm_base_url`: ≤500 chars; when non-empty, MUST start with `http://` or `https://`.
- `support_chat_llm_api_key`: 1..500 chars when non-empty.
- When `support_chat_llm_enabled = true`: both `support_chat_llm_base_url` AND `support_chat_llm_api_key` MUST be non-empty (where empty means: not present in the request AND no prior stored value exists, OR present-and-empty in the request).
- `support_chat_faqs`: each entry's `question` 1..200 chars, `answer` 1..5000 chars, max 50 entries.

#### Scenario: Defaults on a fresh install

- **GIVEN** a clean database
- **WHEN** the public settings endpoint is queried
- **THEN** `support_chat_enabled = false`, `support_chat_anonymous_llm = false`, `support_chat_excluded_routes = ["/payment","/purchase","/admin/*"]`

#### Scenario: Enabling LLM without credentials is rejected

- **GIVEN** the stored values `support_chat_llm_base_url = ""` and `support_chat_llm_api_key = ""`
- **WHEN** the admin PUTs `support_chat_llm_enabled = true` without supplying the two new fields
- **THEN** the save fails with `400 INVALID_SUPPORT_CHAT_LLM_CREDENTIALS` and the previous values (including `support_chat_llm_enabled`) are retained

#### Scenario: Disabling LLM keeps widget alive

- **GIVEN** `support_chat_enabled = true` and admin sets `support_chat_llm_enabled = false`
- **WHEN** the user opens the widget
- **THEN** the widget renders FAQs and the "Submit a ticket" button, but the chat input is disabled with a notice "Self-service chat is temporarily unavailable"

### Requirement: RAG Injection in Chat Prompt Assembly

The chat handler SHALL extend its system-prompt assembly with a RAG section when `support_chat_rag_enabled = true`. Specifically, before invoking the upstream LLM the system SHALL:

1. Take the **most recent** user message from the request body.
2. Compute its embedding by calling `<support_chat_llm_base_url>/embeddings` with `Authorization: Bearer <support_chat_llm_api_key>` and `model = support_chat_rag_embed_model`. The embedding helper SHALL share the same credential pair as the chat endpoint (no separate api_key_id resolution).
3. Retrieve top-K knowledge entries via the vector retrieval helper defined by the `support-knowledge-rag` capability (K = `support_chat_rag_top_k`).
4. If retrieval returns one or more entries, format them as a `## 相关知识` (or `## Relevant Knowledge`) Markdown section, with each entry prefixed by `[FAQ]` (for FAQ items) or `[DOC]` (for doc chunks, including the `source_url` for attribution), and inject the section between the admin-configured prompt and the hardcoded safety footer.
5. If retrieval returns zero entries, the prompt SHALL be assembled exactly as defined in the SSE Chat Endpoint requirement above (no `## 相关知识` section emitted, avoiding noise to the LLM).

When `support_chat_rag_enabled = false`, the chat handler SHALL behave exactly as defined by the SSE Chat Endpoint requirement above with no embedding call performed. This preserves the ability to fully disable RAG (and skip its embedding cost) via a single setting toggle.

The system SHALL impose a **token budget** on the RAG section: the total characters of `[FAQ]/[DOC]` content SHALL NOT exceed `support_chat_max_request_tokens × 2 × 0.5` characters (using the project's char≈0.5 tokens estimate). When the budget is exceeded, lower-similarity entries SHALL be dropped first until the budget is met. The truncation SHALL prefer to keep at least one entry rather than emitting an empty section.

The pre-flight rules above (auth check, rate limiting, **configuration check**, turn truncation, token cap) SHALL execute **before** the RAG injection. Token-cap-driven truncation of historical messages SHALL apply to the assembled prompt **including** the RAG section.

#### Scenario: RAG enabled and retrieval succeeds

- **GIVEN** `support_chat_rag_enabled = true`, retrieval returns 3 entries (1 FAQ + 2 docs) above threshold
- **WHEN** an authenticated user sends a chat message
- **THEN** the assembled system prompt contains a `## 相关知识` section with the 3 entries (FAQ first or interleaved by score), each entry prefixed with `[FAQ]` or `[DOC]`, the doc entries include their `source_url` (e.g. `[DOC] (来源: https://docs.example/keys)`); the section appears between the admin prompt and the safety footer

#### Scenario: RAG enabled but no relevant knowledge

- **GIVEN** `support_chat_rag_enabled = true`, retrieval returns zero entries (all below 0.3 threshold)
- **WHEN** an authenticated user sends a chat message
- **THEN** the assembled system prompt does NOT contain a `## 相关知识` section; the prompt structure is `<admin_prompt>\n\n<safety_footer>`

#### Scenario: RAG disabled bypasses embedding entirely

- **GIVEN** `support_chat_rag_enabled = false`
- **WHEN** an authenticated user sends a chat message
- **THEN** no embedding call is made AND the assembled prompt matches the non-RAG structure exactly (no `## 相关知识` section)

#### Scenario: Embedding service failure does not break chat

- **GIVEN** `support_chat_rag_enabled = true` and the upstream `<base_url>/embeddings` returns 5xx for the user message
- **WHEN** the chat request is processed
- **THEN** the chat request still succeeds (SSE response streams normally); the assembled prompt does NOT contain a `## 相关知识` section; the embedding error is logged but not surfaced to the user

#### Scenario: RAG section honors token budget

- **GIVEN** `support_chat_rag_enabled = true`, `support_chat_max_request_tokens = 4000`, retrieval returned 5 entries totalling ~6000 chars (estimated ~3000 tokens, well over the 0.5 × budget allocation)
- **WHEN** the prompt is assembled
- **THEN** the lowest-similarity entries are dropped from the section until the section's token estimate ≤ `support_chat_max_request_tokens × 0.5`, and at least the highest-similarity entry remains

### Requirement: FAQ Source of Truth Migration

The chat handler's FAQ click behavior (defined by the "Public FAQ Endpoint" requirement above) SHALL continue to surface FAQ items to the widget, but the data source SHALL be `support_faq_items` (the new table owned by the `support-knowledge-rag` capability) rather than the legacy `support_chat_faqs` setting. The public FAQ endpoint `GET /api/v1/support/chat/faqs` SHALL return entries from `support_faq_items` where `enabled = true`, ordered by `sort_order ASC, id ASC`.

When `support_faq_items` is empty AND the legacy setting is non-empty (e.g. immediately before the migration runs, or in an edge-case where the migration is delayed), the system MAY fall back to reading the legacy setting, but the **persistent** source of truth SHALL be the table.

#### Scenario: Public FAQ endpoint reads from new table

- **GIVEN** `support_faq_items` contains 3 rows with `enabled = true` and 1 with `enabled = false`
- **WHEN** an anonymous client GETs `/api/v1/support/chat/faqs`
- **THEN** the response contains exactly the 3 enabled rows in `sort_order` order; the legacy `support_chat_faqs` setting (regardless of its content) is not consulted

### Requirement: External LLM Credential Settings

The system SHALL expose two new admin-only settings that together constitute the credentials for the upstream LLM/embedding provider used by the support-chat widget and the support-knowledge-rag pipeline:

- `support_chat_llm_base_url` (string, default `""`): the OpenAI-compatible HTTP base URL. MUST be ≤500 characters and MUST start with `http://` or `https://`. Examples: `https://api.openai.com/v1`, `https://api.deepseek.com/v1`, `https://<resource>.openai.azure.com/openai/deployments/<deployment>`.
- `support_chat_llm_api_key` (string, default `""`): the bearer token. MUST be 1..500 characters when non-empty.

When `support_chat_llm_enabled = true`, both fields MUST be non-empty; otherwise the settings PUT endpoint SHALL reject the change with HTTP 400 `INVALID_SUPPORT_CHAT_LLM_CREDENTIALS`. When `support_chat_llm_enabled = false`, the fields MAY be empty and the validation rule SHALL NOT apply.

The system SHALL handle `support_chat_llm_api_key` as a secret:

- The admin GET response SHALL replace the cleartext value with a masked sentinel: when the stored value is ≥4 characters, the sentinel is `"sk-***" + last4(value)`; when shorter, the sentinel is `"***"`. When the stored value is empty, the response field is `""`.
- On admin UPDATE, when the request body's `support_chat_llm_api_key` equals the masked sentinel returned by the most recent GET, the system SHALL treat it as "leave unchanged" and SHALL NOT overwrite the stored value. Any other value (including `""`) SHALL be written verbatim.

Neither field SHALL appear in the public settings payload; both are admin-internal.

#### Scenario: Defaults on a fresh install

- **GIVEN** a clean database
- **WHEN** the admin GET /api/admin/settings is called
- **THEN** `support_chat_llm_base_url = ""`, `support_chat_llm_api_key = ""`, and `support_chat_llm_enabled = false`

#### Scenario: API key is masked in admin GET response

- **GIVEN** the stored value is `sk-abc123def456ghi789xyz`
- **WHEN** an admin GETs the settings
- **THEN** the response field `support_chat_llm_api_key` equals `sk-***9xyz` (the cleartext is never returned)

#### Scenario: Leave-unchanged on PUT with masked sentinel

- **GIVEN** the stored value is `sk-secret-abc-real-token-xyz`, the admin's last GET response contained the mask `sk-***ttxz` (or whatever last4 it computed)
- **WHEN** the admin PUTs settings with `support_chat_llm_api_key` equal to that exact masked sentinel
- **THEN** the stored cleartext value is unchanged

#### Scenario: Empty key when LLM enabled is rejected

- **GIVEN** `support_chat_llm_enabled = true` is being set in a single PUT
- **WHEN** the request body has `support_chat_llm_api_key = ""` AND no prior stored value exists
- **THEN** the PUT returns 400 `INVALID_SUPPORT_CHAT_LLM_CREDENTIALS` and no fields are persisted (atomic save)

#### Scenario: Invalid base_url scheme is rejected

- **GIVEN** any state
- **WHEN** the admin PUTs settings with `support_chat_llm_base_url = "ftp://example/v1"`
- **THEN** the PUT returns 400 `INVALID_SUPPORT_CHAT_LLM_BASE_URL` and no fields are persisted

#### Scenario: Public settings does not leak credentials

- **GIVEN** `support_chat_llm_base_url = "https://api.openai.com/v1"` and `support_chat_llm_api_key = "sk-real-token"`
- **WHEN** an anonymous client GETs the public settings endpoint
- **THEN** the response payload does NOT contain either `support_chat_llm_base_url` or `support_chat_llm_api_key`

### Requirement: External LLM Connection Test Endpoint

The system SHALL expose `POST /api/admin/support/chat/test-llm-connection` (admin-only) accepting a JSON body of:

```
{
  "base_url": string,
  "api_key":  string,
  "model":    string
}
```

The endpoint SHALL issue a single non-streaming `POST <base_url>/chat/completions` with body `{"model": <model>, "messages": [{"role":"user","content":"ping"}], "max_tokens": 1, "stream": false}`, `Authorization: Bearer <api_key>`, `Content-Type: application/json`, and a 5-second total timeout. The endpoint SHALL NOT read or write any setting row; the values are taken from the request body only.

The response SHALL be of shape:

```
{
  "ok":          boolean,
  "latency_ms":  integer,
  "status_code": integer | null,
  "error":       string  | null
}
```

`ok` is `true` iff the upstream returned 2xx within the timeout. On non-2xx, `status_code` SHALL be the upstream HTTP status and `error` SHALL contain a sanitized message (no upstream response body verbatim — only the upstream's `error.message` field if the body parses as JSON, else `"upstream non-2xx"`). On network/timeout error, `status_code` is `null` and `error` describes the failure (`"timeout"`, `"dns_error"`, `"tls_error"`, etc).

If `base_url` is empty, malformed, or has a non-http(s) scheme, the endpoint SHALL return `{ok: false, status_code: null, error: "invalid_base_url"}` without making any network call.

When the request body's `api_key` equals the masked sentinel `sk-***xxxx`, the endpoint SHALL substitute the actual stored cleartext value before issuing the upstream request (so the admin can test the saved credential without re-entering it).

#### Scenario: Successful probe

- **GIVEN** valid OpenAI credentials and `model="gpt-4o-mini"`
- **WHEN** an admin POSTs the test endpoint
- **THEN** the response is `{ok: true, latency_ms: <int>, status_code: 200, error: null}`

#### Scenario: Wrong API key returns 401 details

- **GIVEN** `api_key = "sk-wrong"`, valid base_url and model
- **WHEN** an admin POSTs the test endpoint
- **THEN** the response is `{ok: false, status_code: 401, error: <upstream error message>, latency_ms: <int>}`

#### Scenario: Test with masked sentinel uses stored key

- **GIVEN** the stored cleartext value is `sk-real`, the masked form is `sk-***real`
- **WHEN** the admin POSTs `{base_url: "...", api_key: "sk-***real", model: "..."}`
- **THEN** the upstream request uses `Authorization: Bearer sk-real`

#### Scenario: Invalid base_url short-circuits

- **GIVEN** `base_url = "not a url"`
- **WHEN** an admin POSTs the test endpoint
- **THEN** the response is `{ok: false, status_code: null, error: "invalid_base_url", latency_ms: 0}` and no outbound HTTP request is made

#### Scenario: Non-admin denied

- **GIVEN** a regular authenticated user
- **WHEN** they POST the test endpoint
- **THEN** the response is 403 Forbidden

