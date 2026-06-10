## ADDED Requirements

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

The system SHALL expose `POST /api/v1/support/chat` as a Server-Sent Events endpoint that proxies the client's conversation to the platform's own `/v1/chat/completions` using the admin-configured customer-service API key and model. The endpoint SHALL accept a JSON request body of:

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
3. **Turn truncation**: The system SHALL keep at most `support_chat_max_turns` turns (default 5; configurable 1..20). One "turn" = one user→assistant pair. When the request exceeds the cap, the system SHALL discard the **oldest** message pairs first; the most recent user message SHALL always survive.
4. **Token cap**: The system SHALL estimate the total token count of (system prompt + remaining messages) and, if it exceeds `support_chat_max_request_tokens` (default 16000; configurable 1000..200000), SHALL further drop the oldest non-system messages until the budget is met or only the latest user message remains.
5. **System prompt assembly**: The system prompt SHALL be the concatenation of (a) admin-configured `support_chat_system_prompt`, (b) a hardcoded "platform safety" footer that constrains scope ("only answer questions about {{site_name}}", "do not fabricate", "suggest submitting a ticket when uncertain"). The hardcoded footer SHALL be appended **after** the admin string so it has the final word.
6. **Forwarding**: The request SHALL be forwarded to the platform's own `/v1/chat/completions` with `stream = true`, using the admin-configured `support_chat_api_key_id`'s token as `Authorization`. Token usage and billing SHALL be attributed to the owner of that key (no separate accounting path).

The successful response SHALL stream `text/event-stream` with `data: {choices:[{delta:{content:"..."}}]}` chunks identical to OpenAI's streaming format, terminated by `data: [DONE]`. Upstream errors SHALL be surfaced as `event: error\ndata: {"error":{"message":"...", "type":"..."}}\n\n` and the stream SHALL be closed.

#### Scenario: Authenticated streaming success

- **GIVEN** an authenticated user, `support_chat_enabled = true`, `support_chat_llm_enabled = true`, valid `support_chat_api_key_id`, model `gpt-4o-mini`
- **WHEN** the client POSTs `{session_id:"s1", messages:[{role:"user", content:"hi"}]}`
- **THEN** the response is `200 OK` with `Content-Type: text/event-stream`, the body contains one or more `data: {choices:[{delta:{content:"..."}}]}` chunks followed by `data: [DONE]`

#### Scenario: Anonymous request rejected by default

- **GIVEN** `support_chat_anonymous_llm = false` (the default)
- **WHEN** an anonymous client POSTs to `/api/v1/support/chat`
- **THEN** the response is `401 Unauthorized` (a plain JSON body, not an SSE stream)

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

The remaining settings (`support_chat_title`, `support_chat_welcome`, `support_chat_icon`, `support_chat_llm_enabled`, `support_chat_api_key_id`, `support_chat_model`, `support_chat_system_prompt`, `support_chat_max_turns`, `support_chat_max_request_tokens`, `support_chat_rl_*`, `support_chat_faqs`) SHALL NOT appear in `PublicSettings` (they are admin-internal or fetched via dedicated endpoints).

The system SHALL validate admin updates with the following rules:

- `support_chat_max_turns` ∈ `[1, 20]`.
- `support_chat_max_request_tokens` ∈ `[1000, 200000]`.
- `support_chat_rl_*` ∈ `[1, 100000]`.
- `support_chat_excluded_routes`: each entry 1..200 chars, max 50 entries, no duplicates, must start with `/`.
- `support_chat_api_key_id` (when `support_chat_llm_enabled = true`): MUST reference an existing, enabled API key.
- `support_chat_faqs`: each entry's `question` 1..200 chars, `answer` 1..5000 chars, max 50 entries.

#### Scenario: Defaults on a fresh install

- **GIVEN** a clean database
- **WHEN** the public settings endpoint is queried
- **THEN** `support_chat_enabled = false`, `support_chat_anonymous_llm = false`, `support_chat_excluded_routes = ["/payment","/purchase","/admin/*"]`

#### Scenario: Invalid api_key_id is rejected

- **GIVEN** `support_chat_llm_enabled = true` and admin attempts to save `support_chat_api_key_id = 999999` for a non-existent key
- **THEN** the save fails with a validation error and the previous value is retained

#### Scenario: Disabling LLM keeps widget alive

- **GIVEN** `support_chat_enabled = true` and admin sets `support_chat_llm_enabled = false`
- **WHEN** the user opens the widget
- **THEN** the widget renders FAQs and the "Submit a ticket" button, but the chat input is disabled with a notice "Self-service chat is temporarily unavailable"
