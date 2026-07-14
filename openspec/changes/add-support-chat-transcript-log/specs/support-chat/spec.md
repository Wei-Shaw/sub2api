## ADDED Requirements

### Requirement: Conversation Transcript Persistence

The system SHALL persist every support-chat exchange to durable storage, keyed by the client-supplied `session_id`. All turns sharing a `session_id` SHALL be grouped into a single conversation record. For each request that carries a valid latest user message, the system SHALL append the newest user message and the corresponding assistant reply (the captured upstream response, possibly empty on failure) as message rows under that conversation.

Persistence SHALL be a side effect that never alters the response returned to the client: if a write fails, the system SHALL log a warning and continue serving the user normally.

The system SHALL persist conversations for both authenticated and anonymous users. For authenticated users the conversation SHALL record `user_id` and `client_ip`; for anonymous users it SHALL record `client_ip` and leave `user_id` empty.

#### Scenario: First turn creates a conversation

- **GIVEN** support chat LLM is enabled and configured
- **WHEN** a user sends the first message with `session_id = "s-1"` and the upstream replies successfully
- **THEN** a conversation with `session_id = "s-1"`, `turn_count = 1`, `last_status = success` is created
- **AND** two message rows are stored: one `role = user` with the sent text, one `role = assistant` with the accumulated reply

#### Scenario: Subsequent turn merges into the same conversation

- **GIVEN** a conversation `session_id = "s-1"` already exists with `turn_count = 1`
- **WHEN** the same session sends a second message and the upstream replies successfully
- **THEN** no new conversation is created; the existing conversation is updated to `turn_count = 2` with a refreshed `last_at`
- **AND** only the newest user message and the new assistant reply are appended (earlier history is not re-stored)

#### Scenario: Anonymous conversation stores IP without user

- **GIVEN** `support_chat_anonymous_llm = true` and the request has no auth token
- **WHEN** an anonymous user sends a message
- **THEN** the conversation records `client_ip` and leaves `user_id` empty

#### Scenario: Persistence failure does not break the response

- **GIVEN** the transcript store is unreachable
- **WHEN** a user sends a message and the upstream replies successfully
- **THEN** the user still receives the full streamed reply
- **AND** the persistence failure is logged, not surfaced to the client

### Requirement: Streamed Reply Capture Without Latency Impact

While proxying the upstream SSE stream to the client, the system SHALL tee the stream: it SHALL write each chunk to the client first (preserving existing passthrough latency), then parse `choices[].delta.content` from `data:` frames to accumulate the full assistant reply text for persistence. Frames that fail to parse SHALL be skipped silently without interrupting either the passthrough or the accumulation. The terminal `data: [DONE]` frame SHALL mark a normal completion.

#### Scenario: Successful stream captures full reply

- **GIVEN** the upstream emits delta frames `"Hel"`, `"lo"`, then `data: [DONE]`
- **WHEN** the stream completes
- **THEN** the client receives all frames unchanged
- **AND** the stored assistant message content equals `"Hello"` with status `success`

#### Scenario: Unparseable frame does not abort capture

- **GIVEN** the upstream emits a malformed `data:` frame between two valid delta frames
- **WHEN** the stream completes
- **THEN** the malformed frame is passed through to the client unchanged
- **AND** the accumulated reply contains the content of the two valid frames

### Requirement: Per-Turn Status Classification

Each assistant message row SHALL record a status drawn from the full taxonomy: `success`, `upstream_auth`, `upstream_error`, `interrupted`, `rate_limited`, `config_error`. When a status other than `success` applies, the row SHALL also record a human-readable `error_message`. The owning conversation's `last_status` SHALL reflect the most recent turn's status. All classified turns that carry a valid user message SHALL be persisted, including `rate_limited` and `config_error` (requests rejected before reaching the upstream).

Requests that carry no valid user content — feature disabled, unauthenticated rejection, or malformed/empty `messages` — SHALL NOT be persisted.

#### Scenario: Upstream auth failure is recorded

- **GIVEN** the configured upstream API key is invalid
- **WHEN** a user sends a message and the upstream returns 401
- **THEN** the assistant message row is stored with status `upstream_auth` and a non-empty `error_message`
- **AND** the user message row is still stored

#### Scenario: Client disconnect mid-stream is recorded as interrupted

- **GIVEN** a stream is in progress and has emitted partial content
- **WHEN** the client disconnects before `data: [DONE]`
- **THEN** the assistant message row is stored with status `interrupted` and the partial accumulated text

#### Scenario: Rate-limited request is recorded

- **GIVEN** the per-user rate limit is exceeded
- **WHEN** a user sends a message and receives HTTP 429
- **THEN** a turn is stored with status `rate_limited`, the user message, and an empty assistant reply carrying an `error_message`

#### Scenario: Missing credentials recorded as config_error

- **GIVEN** `support_chat_llm_enabled = true` but `base_url` or `api_key` is empty
- **WHEN** a user sends a message
- **THEN** a turn is stored with status `config_error`

#### Scenario: Feature-disabled request is not persisted

- **GIVEN** `support_chat_enabled = false`
- **WHEN** a request hits the chat endpoint and receives 404
- **THEN** no conversation or message is stored

### Requirement: Admin Conversation Log Viewing

The system SHALL expose admin-only read endpoints to list and inspect stored support-chat conversations, mounted under the existing `/api/v1/admin/support` group (protected by admin authentication). The list endpoint SHALL support pagination and filtering by status, user id, client IP, keyword (matched against message content), and time range, and SHALL NOT return message bodies in the list payload. The detail endpoint SHALL return the conversation header plus the full ordered message timeline.

The admin endpoints SHALL NOT be gated by the `support_chat_enabled` feature flag (admins can inspect historical records at any time).

#### Scenario: Admin lists conversations filtered by status

- **GIVEN** stored conversations with mixed `last_status`
- **WHEN** an admin requests the list with `status = upstream_auth`
- **THEN** only conversations whose latest turn failed with `upstream_auth` are returned, most recent first, without message bodies

#### Scenario: Admin views a full conversation

- **GIVEN** a conversation with three stored turns
- **WHEN** an admin opens its detail by id
- **THEN** the response contains the conversation header and all message rows in chronological order, each with its role, content, status, and error detail

#### Scenario: Non-admin cannot access the log

- **GIVEN** a request without admin authentication
- **WHEN** it hits the admin conversation endpoints
- **THEN** it is rejected by admin authentication

### Requirement: Admin Menu Entry Gated by Support Chat Toggle

The system SHALL add a "客服对话记录 / Support Chat Logs" entry to the admin sidebar navigation. The entry's visibility SHALL follow the public setting `support_chat_enabled`, consistent with how the ticket menu follows `support_ticket_enabled`. No separate enable flag SHALL be introduced for the log view.

#### Scenario: Menu visible when support chat enabled

- **GIVEN** `support_chat_enabled = true`
- **WHEN** an admin views the sidebar
- **THEN** the "客服对话记录" entry is shown and links to the conversation log view

#### Scenario: Menu hidden when support chat disabled

- **GIVEN** `support_chat_enabled = false`
- **WHEN** an admin views the sidebar
- **THEN** the "客服对话记录" entry is not shown
