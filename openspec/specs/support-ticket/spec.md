# Support Ticket

## Purpose

Provide an in-product support ticket system that lets authenticated users submit, view, reply to, and close their own tickets, while giving administrators a managed inbox to filter, reply, prioritize, categorize, and close any ticket. The capability is gated by a master feature flag (`support_ticket_enabled`) so operators can fully hide it without code changes; when disabled, all entry points (sidebar items, public routes) disappear and ticket-creation endpoints return `404 Not Found` to avoid leaking the feature's existence. Existing administrators can still access stored tickets via direct URLs after disablement, ensuring no inflight conversations are lost.
## Requirements
### Requirement: Authenticated Ticket Submission

The system SHALL allow any authenticated (non-anonymous) user to submit a support ticket via `POST /api/v1/support/tickets`. The endpoint SHALL require a valid session/token; anonymous requests SHALL receive `401 Unauthorized` with no ticket created.

The request body SHALL conform to:

```
{
  "title":        string,   // 1..200 chars, required
  "content":      string,   // 1..20000 chars, required, Markdown
  "category":     string,   // required, must equal one of the values in
                            //   admin setting `support_ticket_categories`
  "chat_context": string?   // optional, 0..50000 chars, opaque text
}
```

On success, the system SHALL persist a row in `support_tickets` with `status = "open"`, `priority = <admin default>`, `closed_at = NULL`, server-assigned `id / created_at / updated_at`, and SHALL respond `201 Created` with the full ticket DTO (including a fresh empty `replies` array).

On successful creation, the system SHALL trigger the ticket-notifications side effects synchronously (before returning the response):

1. Insert one `support_ticket_notification` row per resolved admin recipient with `event_type = "ticket_created"`, `recipient_user_id = admin.id`, `actor_user_id = submitter.id`, `title_snapshot`, and `excerpt` (Markdown-stripped first 200 chars of the ticket body).
2. Enqueue `NotificationEmailService.Send(event = "support_ticket.new_ticket", recipient = admin.email, source_type = "support_ticket", source_id = ticket.id, reminder_key = "ticket_created")` for each admin recipient.

The admin recipient set SHALL be resolved by decision γ: if the global setting `ticket_notify_emails` is non-empty, its emails are used; otherwise the system SHALL fall back to all users with `role = "admin"`. Failures in either side effect (notification insert or email enqueue) SHALL NOT roll back the ticket creation — the system SHALL log a warning and return `201` with the created ticket.

When the global setting `support_ticket_enabled` is `false`, the endpoint SHALL respond `404 Not Found` (not `403`), to avoid leaking the existence of the feature.

#### Scenario: Authenticated user creates a ticket

- **GIVEN** an authenticated user `U`, `support_ticket_enabled = true`, `support_ticket_categories = ["充值","账号","API","Bug","其他"]`, default priority `normal`
- **WHEN** `U` issues `POST /api/v1/support/tickets` with `title = "充值未到账"`, `content = "..."`, `category = "充值"`
- **THEN** the response is `201 Created`, the body contains a ticket with `status = "open"`, `priority = "normal"`, `user_id = U.id`, `closed_at = null`, and a row exists in `support_tickets` matching the response

#### Scenario: Anonymous request is rejected

- **WHEN** an anonymous client (no Authorization header) issues `POST /api/v1/support/tickets`
- **THEN** the response is `401 Unauthorized` and no row is inserted

#### Scenario: Category not in admin-configured list

- **GIVEN** `support_ticket_categories = ["充值","账号","API","Bug","其他"]`
- **WHEN** an authenticated user submits a ticket with `category = "其他业务"`
- **THEN** the response is `400 Bad Request` with an error code identifying the invalid category, and no row is inserted

#### Scenario: Feature disabled hides the endpoint

- **GIVEN** `support_ticket_enabled = false`
- **WHEN** an authenticated user issues `POST /api/v1/support/tickets`
- **THEN** the response is `404 Not Found`

#### Scenario: chat_context is accepted opaquely

- **WHEN** an authenticated user submits a ticket whose `chat_context` is a 30 KB string of arbitrary characters (within the 50000-char cap)
- **THEN** the ticket is created and `chat_context` is persisted verbatim and returned in the detail endpoint

#### Scenario: Notifications fan out to admin recipients

- **GIVEN** `ticket_notify_emails = ["ops@example.com"]` and users `A1` (admin, email `a1@x`) and `A2` (admin, email `a2@x`)
- **WHEN** an authenticated user submits a ticket
- **THEN** exactly one `support_ticket_notification` row is inserted per configured admin recipient (in this case one row targeting the internal admin user matched to `ops@example.com` if any; if no match, no notification rows are created but a single email is still enqueued to `ops@example.com`), and the `new_ticket` email is enqueued to `ops@example.com` only

#### Scenario: Fallback to role=admin when setting is empty

- **GIVEN** `ticket_notify_emails = []`, users `A1` and `A2` with `role = "admin"`
- **WHEN** an authenticated user submits a ticket
- **THEN** two `support_ticket_notification` rows are inserted (one each for `A1` and `A2`) and two `new_ticket` emails are enqueued (to `A1.email` and `A2.email`)

#### Scenario: Notification insert failure does not block creation

- **GIVEN** the notification insert path returns an error (e.g. transient DB error)
- **WHEN** the user submits a ticket
- **THEN** the response is still `201 Created` with the ticket persisted, and a warning is logged for the failed notification write

### Requirement: Ticket Owner Read Access

The system SHALL allow an authenticated user to list and read **only** their own tickets. The endpoints `GET /api/v1/support/tickets` and `GET /api/v1/support/tickets/:id` SHALL filter strictly on `user_id = caller.id`. Administrators using the user-facing endpoints SHALL receive the same row-level filtering (i.e. they see only tickets they themselves submitted); to inspect other users' tickets, administrators MUST use the admin endpoints.

The list endpoint SHALL paginate (`page`, `page_size`, `page_size` capped at 100) and SHALL order by `created_at DESC`. The list response SHALL omit the `chat_context` field on each row to avoid bloating list payloads; it is only returned by the detail endpoint.

`GET /api/v1/support/tickets/:id` SHALL, on successful retrieval, upsert a row in `support_ticket_reads` with `(ticket_id, user_id = caller.id, last_read_at = now())` as a side effect. The upsert failure SHALL NOT fail the read (log warning). This causes the caller's unread status on this ticket to clear immediately.

#### Scenario: User lists own tickets

- **GIVEN** users `A` and `B`, each with two tickets
- **WHEN** `A` issues `GET /api/v1/support/tickets`
- **THEN** the response is `200 OK` and contains exactly `A`'s two tickets, ordered newest-first, with no `chat_context` field on any row

#### Scenario: User cannot read another user's ticket

- **GIVEN** ticket `T` with `user_id = B.id`
- **WHEN** user `A` (where `A.id != B.id`) issues `GET /api/v1/support/tickets/T.id`
- **THEN** the response is `404 Not Found` (NOT `403`, to avoid leaking ticket existence)

#### Scenario: Detail endpoint returns chat_context

- **GIVEN** ticket `T` owned by `A` with non-empty `chat_context`
- **WHEN** `A` issues `GET /api/v1/support/tickets/T.id`
- **THEN** the response includes the verbatim `chat_context` value

#### Scenario: Opening detail clears user unread state

- **GIVEN** ticket `T` owned by `A` with an admin reply posted at `t_reply` and no prior `support_ticket_reads` row for `(T, A)`
- **WHEN** `A` issues `GET /api/v1/support/tickets/T.id` at wall-clock `t_read` where `t_read > t_reply`
- **THEN** a `support_ticket_reads` row exists with `(ticket_id=T.id, user_id=A.id, last_read_at ≈ t_read)` and subsequent queries against `GET /api/v1/support/tickets/unread-count` for `A` no longer count `T` as unread

### Requirement: Ticket Replies

The system SHALL allow appending Markdown replies to an open or in-progress ticket via `POST /api/v1/support/tickets/:id/replies` (user) and `POST /api/admin/support/tickets/:id/replies` (admin). Each reply SHALL persist `author_id = caller.id`, `is_admin` snapshotted from the route used (admin route ⇒ `true`, user route ⇒ `false`), `content` (1..20000 chars, required), and a server-assigned `id` and `created_at`.

When the **first** admin reply is posted on a ticket whose `status = "open"`, the system SHALL atomically transition the ticket to `status = "in_progress"`. Subsequent replies (admin or user) SHALL NOT change `status`.

When a ticket has `status = "closed"`, the system SHALL reject any reply attempt (admin or user) with `409 Conflict` and SHALL NOT persist the reply.

On successful reply insertion, the system SHALL trigger ticket-notification side effects synchronously:

- If the reply is an **admin reply**: insert one `support_ticket_notification` row with `recipient_user_id = ticket.user_id`, `event_type = "admin_replied"`, `actor_user_id = admin.id`; enqueue `NotificationEmailService.Send(event = "support_ticket.new_reply", recipient = ticket.owner.email, source_type = "support_ticket", source_id = ticket.id, reminder_key = "admin_replied|<reply.id>")`.
- If the reply is a **user reply**: insert one `support_ticket_notification` row per admin recipient (same resolution as the ticket-creation γ rule) with `event_type = "user_replied"`, `actor_user_id = user.id`; enqueue `new_reply` email to each admin recipient with `reminder_key = "user_replied|<reply.id>"`.

Notification / email failures SHALL NOT roll back the reply insertion (log warnings only).

Additionally, the admin reply route SHALL upsert `support_ticket_reads` for the acting admin (`user_id = admin.id`, `last_read_at = now()`) so that admins do not receive stale unread signals on tickets they just replied to.

#### Scenario: User replies to own open ticket

- **GIVEN** an open ticket `T` owned by user `A`
- **WHEN** `A` issues `POST /api/v1/support/tickets/T.id/replies` with `content = "补充信息..."`
- **THEN** the reply is persisted with `author_id = A.id`, `is_admin = false`; `T.status` remains `"open"`

#### Scenario: First admin reply transitions status

- **GIVEN** an open ticket `T` owned by user `A`, with no admin replies yet
- **WHEN** an admin issues `POST /api/admin/support/tickets/T.id/replies`
- **THEN** the reply is persisted with `is_admin = true` AND `T.status` transitions atomically to `"in_progress"`

#### Scenario: Reply on closed ticket is rejected

- **GIVEN** ticket `T` with `status = "closed"`
- **WHEN** either the owner or an admin attempts to post a reply
- **THEN** the response is `409 Conflict` and no reply is persisted; `T.status` remains `"closed"`

#### Scenario: User cannot reply on another user's ticket

- **GIVEN** ticket `T` with `user_id = B.id`
- **WHEN** user `A` (`A.id != B.id`) issues `POST /api/v1/support/tickets/T.id/replies`
- **THEN** the response is `404 Not Found`

#### Scenario: Admin reply notifies the ticket owner

- **GIVEN** ticket `T` owned by user `U` (email `u@x`)
- **WHEN** admin `M` posts a reply on `T`
- **THEN** a `support_ticket_notification` row exists with `(recipient_user_id=U.id, ticket_id=T.id, event_type="admin_replied", is_read=false)` and a `support_ticket.new_reply` email is enqueued to `u@x`

#### Scenario: User reply notifies admin recipients

- **GIVEN** admins `A1`, `A2` with `role = "admin"` and empty `ticket_notify_emails`
- **WHEN** the ticket owner posts a reply
- **THEN** exactly two `support_ticket_notification` rows are inserted (one each for `A1`, `A2`) with `event_type = "user_replied"` and two `new_reply` emails are enqueued

### Requirement: Ticket Closure

The system SHALL allow both the ticket owner and any admin to close an open or in-progress ticket. The owner uses `POST /api/v1/support/tickets/:id/close`; admins MAY use either that route (when they own the ticket) or `PATCH /api/admin/support/tickets/:id` with `{"status":"closed"}`.

On closure, the system SHALL set `status = "closed"`, `closed_at = now()`, and `updated_at = now()`. A closed ticket is a terminal state: subsequent attempts to close again, change status, or post replies SHALL be rejected with `409 Conflict`.

#### Scenario: Owner closes own ticket

- **GIVEN** an open ticket `T` owned by `A`
- **WHEN** `A` issues `POST /api/v1/support/tickets/T.id/close`
- **THEN** the response is `200 OK`; `T.status = "closed"`, `T.closed_at` is non-null and approximately `now()`

#### Scenario: Reopen attempt is rejected

- **GIVEN** a closed ticket `T`
- **WHEN** an admin issues `PATCH /api/admin/support/tickets/T.id` with `{"status":"open"}`
- **THEN** the response is `409 Conflict` and `T.status` remains `"closed"`

### Requirement: Admin Ticket Management

The system SHALL expose an admin-only set of endpoints under `/api/admin/support/tickets/*` that require `RequireAuth + RequireAdmin`. Non-admin authenticated requests SHALL receive `403 Forbidden`.

The admin list endpoint `GET /api/admin/support/tickets` SHALL accept query parameters `status`, `category`, `priority`, `user_id`, `q` (case-insensitive substring match against `title` and `content`), `page`, and `page_size` (cap 100). It SHALL order results by `priority DESC, created_at DESC`. The list response SHALL omit `chat_context`.

The admin patch endpoint `PATCH /api/admin/support/tickets/:id` SHALL accept any subset of `{status, priority, category}`. Status transitions to `"closed"` SHALL set `closed_at = now()` (as in the closure requirement); transitions away from `"closed"` SHALL be rejected with `409 Conflict`. `priority` MUST be one of `low|normal|high`. `category` MUST equal an entry in the current `support_ticket_categories` setting; historical tickets retain their original category even if the configured list later changes.

`GET /api/admin/support/tickets/:id` SHALL, on successful retrieval, upsert `support_ticket_reads` with `(ticket_id, user_id = admin.id, last_read_at = now())`. Failure SHALL NOT block the read (log warning). This clears the calling admin's unread state on the ticket.

#### Scenario: Non-admin is forbidden

- **GIVEN** a regular authenticated user `U`
- **WHEN** `U` issues `GET /api/admin/support/tickets`
- **THEN** the response is `403 Forbidden`

#### Scenario: Admin filters by status and priority

- **GIVEN** 5 open/high tickets, 3 in-progress/high tickets, 10 closed/normal tickets
- **WHEN** an admin issues `GET /api/admin/support/tickets?status=open&priority=high&page=1&page_size=20`
- **THEN** the response contains exactly the 5 open/high tickets, ordered newest-first

#### Scenario: Admin keyword search

- **GIVEN** tickets with titles `"充值未到账"`, `"API 401 错误"`, `"Bug: 登录页"`
- **WHEN** an admin issues `GET /api/admin/support/tickets?q=未到账`
- **THEN** only the `"充值未到账"` ticket appears in the response

#### Scenario: Admin updates priority

- **GIVEN** ticket `T` with `priority = "normal"`
- **WHEN** an admin issues `PATCH /api/admin/support/tickets/T.id` with `{"priority":"high"}`
- **THEN** the response is `200 OK` and `T.priority = "high"`

#### Scenario: Admin opening detail clears admin unread state

- **GIVEN** ticket `T` with a user reply posted at `t_reply` and no `support_ticket_reads` row for `(T, admin M)`
- **WHEN** admin `M` issues `GET /api/admin/support/tickets/T.id` at `t_read > t_reply`
- **THEN** a `support_ticket_reads` row exists with `(ticket_id=T.id, user_id=M.id, last_read_at ≈ t_read)` and `GET /api/admin/support/tickets/unread-count` for `M` no longer counts `T`

### Requirement: Public Settings Surface and Defaults

The system SHALL expose `support_ticket_enabled` (boolean) in the `PublicSettings` payload returned by `GET /api/settings/public` AND in the SSR `PublicSettingsInjectionPayload`. The two surfaces SHALL stay schema-aligned (covered by `public_settings_injection_schema_test.go`).

The configurable settings SHALL be:

- `support_ticket_enabled` (bool, default `false`): master switch.
- `support_ticket_categories` (string array, default `["充值","账号","API","Bug","其他"]`): non-empty, each item 1..20 chars, max 20 entries, no duplicates.
- `support_ticket_default_priority` (enum `low|normal|high`, default `normal`): default priority for newly created tickets.
- `ticket_notify_emails` (string array of RFC-5322 email addresses, default `[]`, max 20 entries): when non-empty, overrides the admin-role-fallback for ticket notification emails. Each entry MUST be a valid email (validated at admin save time). Duplicate entries SHALL be deduplicated (case-insensitive local-part, per RFC-5321 leniency).

`support_ticket_categories`, `support_ticket_default_priority`, and `ticket_notify_emails` SHALL NOT appear in `PublicSettings` (all admin-internal). The list of currently usable categories SHALL be exposed to authenticated users via `GET /api/v1/support/categories`, returning `{"categories": [...], "default_priority": "..."}`.

#### Scenario: Defaults on a fresh install

- **GIVEN** a clean database with no overrides
- **WHEN** the public settings endpoint is queried
- **THEN** `support_ticket_enabled = false`; the user-facing route `/support/tickets` is hidden from the sidebar; `POST /api/v1/support/tickets` returns `404 Not Found`

#### Scenario: Admin enables the feature

- **GIVEN** an admin updates `support_ticket_enabled = true` and saves
- **WHEN** any client refetches public settings
- **THEN** `support_ticket_enabled = true`; user-facing entries become visible; ticket APIs become reachable

#### Scenario: Empty categories list is rejected

- **WHEN** an admin attempts to save `support_ticket_categories = []`
- **THEN** the save is rejected with a validation error and the previous value is retained

#### Scenario: Category list ordering is preserved

- **GIVEN** an admin saves `support_ticket_categories = ["A","B","C"]`
- **WHEN** the list is read back via `GET /api/v1/support/categories`
- **THEN** the response array is exactly `["A","B","C"]` in the saved order

#### Scenario: ticket_notify_emails validates addresses

- **WHEN** an admin attempts to save `ticket_notify_emails = ["not-an-email"]`
- **THEN** the save is rejected with a validation error and the previous value is retained

#### Scenario: ticket_notify_emails empty falls back to role=admin

- **GIVEN** `ticket_notify_emails = []` and users `A1`, `A2` with `role = "admin"`
- **WHEN** any ticket event fires that requires an admin email fan-out
- **THEN** emails are enqueued to both `A1.email` and `A2.email`

#### Scenario: ticket_notify_emails non-empty overrides admin role

- **GIVEN** `ticket_notify_emails = ["ops@example.com"]` and users `A1`, `A2` with `role = "admin"` and emails `a1@x` / `a2@x`
- **WHEN** any ticket event fires that requires an admin email fan-out
- **THEN** a single email is enqueued to `ops@example.com` and no email is enqueued to `a1@x` or `a2@x`

### Requirement: Chat Widget Handoff to Ticket Form

The ticket creation page (`/support/tickets/new`) SHALL accept the URL query parameters `from=chat` and `session=<localStorage-key>`. When **both** parameters are present, the page SHALL:

1. Read the value at `localStorage[<session>]`. The expected schema is `{ messages: [...], updated_at: string }` (matching the `support-chat` widget's persistence format).
2. Render each message into a Markdown block (with role labels such as `**User:**` / `**Assistant:**`).
3. Pre-fill the `content` Markdown field with the rendered Markdown as an editable draft (the user MAY edit it before submitting).
4. Submit the same rendered Markdown verbatim as the request's `chat_context` field (in addition to whatever final `content` value the user sets).

When the URL query is present but `localStorage[<session>]` is missing, malformed, or empty, the page SHALL silently fall back to an empty draft (logging a `console.warn` for debuggability) and SHALL NOT display an error toast — the URL-driven handoff is best-effort and SHALL never block ticket creation.

When `support_ticket_enabled = false`, this handoff is unreachable because the ticket creation route itself is hidden / 404s.

#### Scenario: Handoff fills both content and chat_context

- **GIVEN** `localStorage.support_chat_session_v1 = {messages:[{role:"user",content:"怎么充值？"},{role:"assistant",content:"...步骤..."}], updated_at:"..."}` and the ticket creation page is opened at `/support/tickets/new?from=chat&session=support_chat_session_v1`
- **WHEN** the page renders
- **THEN** the `content` textarea is pre-filled with a Markdown rendering of the two messages, the user can edit it, AND clicking submit results in `POST /api/v1/support/tickets` with `chat_context` equal to the original Markdown rendering (not the user-edited content)

#### Scenario: Missing localStorage falls back to empty draft

- **GIVEN** `localStorage.support_chat_session_v1` is undefined (or empty messages) AND the page is opened at `/support/tickets/new?from=chat&session=support_chat_session_v1`
- **WHEN** the page renders
- **THEN** the `content` textarea is empty, no error toast appears, and a `console.warn` is emitted; submission proceeds normally without `chat_context`

#### Scenario: Handoff URL absent leaves form blank

- **GIVEN** the page is opened at `/support/tickets/new` (no query parameters)
- **WHEN** the page renders
- **THEN** the form is fully blank and `localStorage` is not read at all

