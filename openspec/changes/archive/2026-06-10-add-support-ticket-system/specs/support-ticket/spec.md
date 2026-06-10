## ADDED Requirements

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

### Requirement: Ticket Owner Read Access

The system SHALL allow an authenticated user to list and read **only** their own tickets. The endpoints `GET /api/v1/support/tickets` and `GET /api/v1/support/tickets/:id` SHALL filter strictly on `user_id = caller.id`. Administrators using the user-facing endpoints SHALL receive the same row-level filtering (i.e. they see only tickets they themselves submitted); to inspect other users' tickets, administrators MUST use the admin endpoints.

The list endpoint SHALL paginate (`page`, `page_size`, `page_size` capped at 100) and SHALL order by `created_at DESC`. The list response SHALL omit the `chat_context` field on each row to avoid bloating list payloads; it is only returned by the detail endpoint.

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

### Requirement: Ticket Replies

The system SHALL allow appending Markdown replies to an open or in-progress ticket via `POST /api/v1/support/tickets/:id/replies` (user) and `POST /api/admin/support/tickets/:id/replies` (admin). Each reply SHALL persist `author_id = caller.id`, `is_admin` snapshotted from the route used (admin route ⇒ `true`, user route ⇒ `false`), `content` (1..20000 chars, required), and a server-assigned `id` and `created_at`.

When the **first** admin reply is posted on a ticket whose `status = "open"`, the system SHALL atomically transition the ticket to `status = "in_progress"`. Subsequent replies (admin or user) SHALL NOT change `status`.

When a ticket has `status = "closed"`, the system SHALL reject any reply attempt (admin or user) with `409 Conflict` and SHALL NOT persist the reply.

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

### Requirement: Public Settings Surface and Defaults

The system SHALL expose `support_ticket_enabled` (boolean) in the `PublicSettings` payload returned by `GET /api/settings/public` AND in the SSR `PublicSettingsInjectionPayload`. The two surfaces SHALL stay schema-aligned (covered by `public_settings_injection_schema_test.go`).

The configurable settings SHALL be:

- `support_ticket_enabled` (bool, default `false`): master switch.
- `support_ticket_categories` (string array, default `["充值","账号","API","Bug","其他"]`): non-empty, each item 1..20 chars, max 20 entries, no duplicates.
- `support_ticket_default_priority` (enum `low|normal|high`, default `normal`): default priority for newly created tickets.

`support_ticket_categories` and `support_ticket_default_priority` SHALL NOT appear in `PublicSettings` (they are admin-internal). The list of currently usable categories SHALL be exposed to authenticated users via `GET /api/v1/support/categories`, returning `{"categories": [...], "default_priority": "..."}`.

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
