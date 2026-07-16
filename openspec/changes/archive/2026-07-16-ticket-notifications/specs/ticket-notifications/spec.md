## ADDED Requirements

### Requirement: Ticket Read State Tracking

The system SHALL persist per-user read state for tickets in a dedicated `support_ticket_reads` table with the following schema:

```
support_ticket_reads
─────────────────────────────────────────────────
id             bigserial PRIMARY KEY
ticket_id      int8 NOT NULL REFERENCES support_tickets(id) ON DELETE CASCADE
user_id        int8 NOT NULL REFERENCES users(id) ON DELETE CASCADE
last_read_at   timestamptz NOT NULL
created_at     timestamptz NOT NULL DEFAULT now()
updated_at     timestamptz NOT NULL DEFAULT now()

UNIQUE (ticket_id, user_id)
INDEX (user_id, last_read_at DESC)
```

The upsert semantics SHALL be: on `(ticket_id, user_id)` conflict, update `last_read_at = EXCLUDED.last_read_at`, `updated_at = now()`.

Absence of a row for `(ticket_id, user_id)` SHALL be interpreted as `last_read_at = '1970-01-01'` (i.e. never read) — this is the default state for all users on all tickets until they open the ticket.

Rows SHALL be created lazily by ticket-detail endpoints (see `support-ticket` capability updates) and by admin reply endpoints; there is no explicit "mark as read" endpoint for tickets.

Ticket unread predicates SHALL be:

- **User unread on ticket `T`** ⇔ `T.user_id = user.id` AND exists `r ∈ T.replies` where `r.is_admin = true` AND `r.created_at > coalesce(read.last_read_at, '1970-01-01')`.
- **Admin unread on ticket `T`** ⇔ (`T.created_at > coalesce(read.last_read_at, '1970-01-01')`) OR (exists `r ∈ T.replies` where `r.is_admin = false` AND `r.created_at > coalesce(read.last_read_at, '1970-01-01')`).

#### Scenario: First-time reader has unread by default

- **GIVEN** a ticket `T` with an admin reply and no `support_ticket_reads` row for `(T, U)`
- **WHEN** the user-unread predicate is evaluated for `U`
- **THEN** `T` is considered unread for `U`

#### Scenario: Upsert clears unread instantly

- **GIVEN** `T` with an admin reply at `t_reply` and a read row `(T, U, last_read_at = t_read)` where `t_read > t_reply`
- **WHEN** the user-unread predicate is evaluated for `U`
- **THEN** `T` is NOT considered unread for `U`

#### Scenario: New reply after read re-triggers unread

- **GIVEN** a read row `(T, U, last_read_at = t_read)` and admin then posts a new reply at `t_new > t_read`
- **WHEN** the user-unread predicate is re-evaluated for `U`
- **THEN** `T` is again considered unread for `U`

#### Scenario: Ticket deletion cascades read rows

- **GIVEN** ticket `T` and read rows for `(T, U1)`, `(T, U2)`
- **WHEN** `T` is hard-deleted from `support_tickets`
- **THEN** all `support_ticket_reads` rows referencing `T` are removed by the FK cascade

### Requirement: Ticket Notification Records

The system SHALL persist ticket notification events in a dedicated `support_ticket_notification` table:

```
support_ticket_notification
─────────────────────────────────────────────────
id                bigserial PRIMARY KEY
recipient_user_id int8 NOT NULL REFERENCES users(id) ON DELETE CASCADE
ticket_id         int8 NOT NULL REFERENCES support_tickets(id) ON DELETE CASCADE
event_type        varchar(32) NOT NULL   -- 'ticket_created' | 'admin_replied' | 'user_replied'
title_snapshot    varchar(200) NOT NULL  -- redundant copy of ticket title at event time
excerpt           varchar(500) NOT NULL  -- Markdown-stripped first 200 chars of the triggering text
actor_user_id     int8 NULL REFERENCES users(id) ON DELETE SET NULL
is_read           bool NOT NULL DEFAULT false
created_at        timestamptz NOT NULL DEFAULT now()
read_at           timestamptz NULL

INDEX (recipient_user_id, is_read, created_at DESC)
```

The `excerpt` field SHALL be computed by:
1. Stripping Markdown syntax (headings, links, images, code fences) using a conservative regex-based sanitizer.
2. Collapsing consecutive whitespace to a single space.
3. Truncating to at most 200 UTF-8 characters (not bytes), appending `…` if truncated.

For `event_type`:

- `ticket_created` — inserted when a user creates a ticket; excerpt is derived from `ticket.content`.
- `admin_replied` — inserted when an admin posts a reply; excerpt is derived from `reply.content`.
- `user_replied` — inserted when a user posts a reply on their own ticket; excerpt is derived from `reply.content`.

If insertion fails at write time, the caller (ticket service) SHALL log a warning and continue — the error MUST NOT propagate to the ticket creation / reply flow.

#### Scenario: Excerpt truncates Markdown

- **GIVEN** a reply with content `"# 标题\n\n这是一段**加粗**说明，" + "字" * 300`
- **WHEN** the excerpt is generated
- **THEN** the resulting string starts with `"标题 这是一段加粗说明，"` (Markdown syntax stripped, whitespace collapsed) and is at most 200 characters long, ending with `"…"` if truncated

#### Scenario: Notification recipients follow event routing

- **GIVEN** ticket `T` owned by user `U`, admins `A1`, `A2` matched by γ rule
- **WHEN** admin `A1` posts a reply on `T`
- **THEN** exactly one notification row is inserted with `(recipient_user_id=U.id, event_type="admin_replied", actor_user_id=A1.id)` — no rows are inserted for `A1` or `A2`

#### Scenario: Ticket deletion cascades notifications

- **GIVEN** ticket `T` with three notification rows across different recipients
- **WHEN** `T` is hard-deleted from `support_tickets`
- **THEN** all three notification rows are removed by the FK cascade

### Requirement: Ticket Notification Email Events

The system SHALL register two new notification email event types in `NotificationEmailService`:

- `support_ticket.new_ticket` — sent to admin recipients when a new ticket is created. Variables: `TicketID`, `Title`, `Excerpt`, `ActorName` (the submitter's display name or email), `PortalURL` (absolute URL to `/admin/support/tickets/:id`).
- `support_ticket.new_reply` — sent when a reply is posted. Recipient depends on direction: admin reply → ticket owner; user reply → admin recipients. Variables: `TicketID`, `Title`, `Excerpt`, `ActorName`, `IsAdminReply` (bool for template branching), `PortalURL` (owner path `/support/tickets/:id` or admin path `/admin/support/tickets/:id`).

Both events SHALL ship with official HTML + plaintext templates in `zh` and `en`. Other locales SHALL fall back to `en` (matching existing NotificationEmailService behavior).

Each `Send` invocation SHALL use `SourceType = "support_ticket"`, `SourceID = strconv.FormatInt(ticket.id, 10)`, and:
- `ReminderKey = "ticket_created"` for the new-ticket event,
- `ReminderKey = "admin_replied|<reply.id>"` or `"user_replied|<reply.id>"` for reply events.

This ReminderKey structure guarantees per-reply idempotency: a network retry that re-invokes `Send` with the same key SHALL NOT deliver a duplicate email (the existing `deliveryKey` dedupe path in `notification_email_service.go` handles this).

Email dispatch failures SHALL NOT roll back the underlying ticket / reply operation (log warning only).

#### Scenario: Duplicate send is deduped

- **GIVEN** a reply `R` on ticket `T` and the system attempts to enqueue `new_reply` with `ReminderKey = "admin_replied|R.id"` twice
- **WHEN** both enqueues complete
- **THEN** only one email is actually sent to the recipient (deduped by the delivery key)

#### Scenario: Admin recipient resolution uses γ rule

- **GIVEN** `ticket_notify_emails = ["ops@example.com"]`
- **WHEN** a new ticket is created
- **THEN** the `new_ticket` email is enqueued exactly once with recipient `ops@example.com` and NOT enqueued to any user having `role = "admin"`

#### Scenario: Empty override falls back to role=admin

- **GIVEN** `ticket_notify_emails = []`, users `A1`, `A2` with `role = "admin"`
- **WHEN** a new ticket is created
- **THEN** two `new_ticket` emails are enqueued — one to `A1.email` and one to `A2.email`

#### Scenario: Template variables render

- **GIVEN** ticket `T` with `id=42`, `title="充值未到账"`, `content="..."`, submitter `U` with display name `"张三"`
- **WHEN** the `new_ticket` email is rendered in `zh`
- **THEN** the rendered email body contains `"#42"` or `"42"`, `"充值未到账"`, `"张三"`, and a clickable link ending with `/admin/support/tickets/42`

### Requirement: Ticket Unread REST API (User)

The system SHALL expose the following user-facing REST endpoints under `/api/v1/support/tickets/`. All SHALL require authentication (`RequireAuth`) and SHALL be gated by `support_ticket_enabled` (returning `404 Not Found` when disabled).

- `GET /api/v1/support/tickets/unread-count` — Returns `{ "count": <int> }` where `count` is the number of tickets owned by the caller that satisfy the user-unread predicate (see Ticket Read State Tracking).
- `GET /api/v1/support/tickets/notifications?page=1&page_size=20` — Returns a paginated list of the caller's own notification records (`recipient_user_id = caller.id`), ordered by `created_at DESC`. Response shape:
  ```
  {
    "items": [ { "id": int64, "ticket_id": int64, "event_type": string,
                  "title_snapshot": string, "excerpt": string,
                  "actor_name": string?, "is_read": bool, "created_at": string } ],
    "total": int,
    "unread_count": int,
    "page": int,
    "page_size": int
  }
  ```
  `page_size` SHALL be capped at 100.
- `POST /api/v1/support/tickets/notifications/:id/read` — Marks a single notification as read (`is_read = true`, `read_at = now()`). The system SHALL check that the notification's `recipient_user_id = caller.id`; on mismatch, return `404 Not Found` (not `403`, to avoid leaking existence).
- `POST /api/v1/support/tickets/notifications/read-all` — Marks all of the caller's unread notifications as read.

Idempotency: marking an already-read notification as read again SHALL succeed (`200 OK`) without modifying `read_at`.

#### Scenario: Unread count reflects predicate

- **GIVEN** caller `U` owns tickets `T1` (has unread admin reply), `T2` (all replies read), `T3` (has unread admin reply)
- **WHEN** `U` issues `GET /api/v1/support/tickets/unread-count`
- **THEN** the response is `200 OK` with body `{"count": 2}`

#### Scenario: Notification list is caller-scoped

- **GIVEN** notifications exist with `recipient_user_id` in `{U, U, V}`
- **WHEN** `U` issues `GET /api/v1/support/tickets/notifications`
- **THEN** the response contains exactly the two records where `recipient_user_id = U.id`; the record for `V` is absent

#### Scenario: Mark-read on foreign notification is 404

- **GIVEN** notification `N` with `recipient_user_id = V.id`
- **WHEN** `U` (`U.id != V.id`) issues `POST /api/v1/support/tickets/notifications/N.id/read`
- **THEN** the response is `404 Not Found` and `N.is_read` remains unchanged

#### Scenario: Mark-all-read clears list

- **GIVEN** caller `U` has 5 unread notifications and 3 read notifications
- **WHEN** `U` issues `POST /api/v1/support/tickets/notifications/read-all`
- **THEN** the response is `200 OK`, all 8 notifications now have `is_read = true`, and a subsequent `GET /api/v1/support/tickets/notifications` returns `unread_count = 0`

#### Scenario: Feature disabled hides endpoints

- **GIVEN** `support_ticket_enabled = false`
- **WHEN** the caller issues any of the four endpoints
- **THEN** each returns `404 Not Found`

### Requirement: Ticket Unread REST API (Admin)

The system SHALL expose the following admin-facing endpoints under `/api/admin/support/tickets/`. All SHALL require `RequireAuth + RequireAdmin` (non-admin ⇒ `403 Forbidden`) and be gated by `support_ticket_enabled` (⇒ `404 Not Found` when disabled).

- `GET /api/admin/support/tickets/unread-count` — Returns `{ "count": <int> }`, count = number of tickets satisfying the admin-unread predicate **for the calling admin** (each admin's own unread count, independent of other admins).
- `GET /api/admin/support/tickets/notifications?page=1&page_size=20` — Returns notifications with `recipient_user_id = admin.id`, same response shape as the user endpoint.
- `POST /api/admin/support/tickets/notifications/:id/read` — Marks one notification as read; 404 if the notification's `recipient_user_id` does not match the calling admin.
- `POST /api/admin/support/tickets/notifications/read-all` — Marks all of the calling admin's unread notifications as read.

Note that these are strictly per-admin: admin A marking as read does NOT affect admin B.

#### Scenario: Unread counts are per-admin

- **GIVEN** admins `A1` and `A2`, both admin-recipients for ticket `T`; `A1` has opened `T` but `A2` has not
- **WHEN** each queries `GET /api/admin/support/tickets/unread-count`
- **THEN** `A1` receives `{"count": 0}` and `A2` receives `{"count": 1}` (assuming no other tickets)

#### Scenario: One admin marking a notification read leaves others unaffected

- **GIVEN** notifications `N1` (recipient `A1`) and `N2` (recipient `A2`) both referencing the same ticket event
- **WHEN** `A1` issues `POST /api/admin/support/tickets/notifications/N1.id/read`
- **THEN** `N1.is_read = true` and `N2.is_read` remains `false`

### Requirement: Announcement Bell Tabbed Layout

The frontend `AnnouncementBell.vue` component SHALL present two tabs: **公告 (Announcements)** and **工单 (Tickets)**. The bell icon SHALL display a single unread badge whose count is the sum of the announcement store's `unreadCount` and the ticket unread store's `notificationsUnreadCount`.

The default active tab when the panel opens SHALL follow this rule:

1. If announcement unread > 0 AND ticket unread == 0 → announcement tab is active.
2. If ticket unread > 0 AND announcement unread == 0 → ticket tab is active.
3. Otherwise (both > 0, or both == 0) → announcement tab is active.

Each tab SHALL show its own unread count as an inline badge in the tab header (e.g. `工单 (3)`), suppressed when that tab's count is `0`.

The Ticket tab SHALL render notification items from `useTicketUnreadStore.notifications` (paginated). Each item SHALL show:
- Icon inferred from `event_type` (new ticket / admin replied / user replied).
- `title_snapshot` prominently.
- `excerpt` on a secondary line, single-line-clipped.
- Relative time (`created_at`, formatted via existing i18n time helpers).
- Visual "unread" state (highlighted background + bold title) when `is_read = false`.

Clicking a ticket notification item SHALL:
1. Enqueue a `markNotificationRead(item.id)` call (fire-and-forget; UI marks it read optimistically).
2. Navigate to `/support/tickets/:ticket_id` for regular users, or `/admin/support/tickets/:ticket_id` for admin users (route selection based on the current user's role from `useAuthStore`).
3. Close the bell panel.

The Ticket tab SHALL include a "全部标为已读 / Mark all as read" action, which calls `markAllNotificationsRead()` and re-fetches the list.

The Announcement tab SHALL preserve **all existing behavior** — layout, list content, mark-read semantics, and API calls (`announcements.ts` store) — with zero regression. Its logic SHOULD be extracted into a child component (e.g. `AnnouncementTabPanel.vue`) for maintainability, but this refactoring MUST NOT change any user-visible behavior or the announcement store's public API.

#### Scenario: Both tabs empty shows announcement tab

- **GIVEN** `announcementsStore.unreadCount = 0` and `ticketUnreadStore.notificationsUnreadCount = 0`
- **WHEN** the user opens the bell
- **THEN** the Announcement tab is the active tab and the badge count is `0` (badge hidden)

#### Scenario: Only ticket has unread opens ticket tab

- **GIVEN** `announcementsStore.unreadCount = 0` and `ticketUnreadStore.notificationsUnreadCount = 5`
- **WHEN** the user opens the bell
- **THEN** the Ticket tab is active, the bell badge shows `5`, and the tab header shows `工单 (5)`

#### Scenario: Both tabs unread defaults to announcement

- **GIVEN** `announcementsStore.unreadCount = 2` and `ticketUnreadStore.notificationsUnreadCount = 3`
- **WHEN** the user opens the bell
- **THEN** the Announcement tab is active and the bell badge shows `5`

#### Scenario: Clicking a ticket item marks read and navigates

- **GIVEN** a regular (non-admin) user with an unread ticket notification `N` referring to `ticket_id = 42`
- **WHEN** the user clicks `N` in the ticket tab
- **THEN** `POST /api/v1/support/tickets/notifications/42/read` is fired, the panel closes, and the browser navigates to `/support/tickets/42`

#### Scenario: Admin click routes to admin URL

- **GIVEN** an admin user with an unread ticket notification `N` referring to `ticket_id = 42`
- **WHEN** the admin clicks `N`
- **THEN** the browser navigates to `/admin/support/tickets/42`

#### Scenario: Announcement tab regression-free

- **GIVEN** an existing announcement `X` visible to all users
- **WHEN** a user opens the bell and interacts with the announcement tab (view / mark as read / dismiss)
- **THEN** all existing announcement behaviors work identically to before this change; the announcement store's public methods are unchanged

### Requirement: Sidebar Ticket Red Dot

The user-facing sidebar (`AppSidebar.vue`) SHALL render a red dot next to the "工单 / My Support" menu item when `useTicketUnreadStore().unreadCount > 0`. The admin-facing sidebar SHALL render the same red dot on the admin "工单管理 / Support Tickets" menu item using the admin-scope of the same store.

`useTicketUnreadStore().unreadCount` SHALL reflect the server's ticket-unread predicate (i.e. `GET .../unread-count`), refreshed by:
1. On store initialization / login (with a 3s delay to defer initial paint).
2. On every route change (`router.afterEach`), debounced to at most one refresh per 5 seconds.
3. On `visibilitychange` from `hidden` → `visible`.
4. On a background timer with 60-second interval.
5. After any successful `POST` to reply / close / mark-read endpoints (imperative refresh).

The red dot SHALL disappear immediately (optimistically) after the user opens a ticket detail page (once the client observes the corresponding `unread-count` refresh return a new lower value).

#### Scenario: Dot appears when unread > 0

- **GIVEN** the user has 2 tickets with unread admin replies
- **WHEN** the sidebar is rendered
- **THEN** the "工单" menu item shows the red dot

#### Scenario: Dot disappears after reading

- **GIVEN** the red dot is visible and the user opens the only unread ticket
- **WHEN** the ticket detail page loads (which triggers `unread-count` re-fetch)
- **THEN** the sidebar red dot is gone

#### Scenario: Polling recovers stale state

- **GIVEN** the user has the app open with no unread tickets, then an admin posts a reply
- **WHEN** at most 60 seconds elapse without any user interaction
- **THEN** the sidebar red dot appears on the "工单" menu item without requiring page reload

### Requirement: Ticket Unread Store (Frontend)

The frontend SHALL provide a pinia store `useTicketUnreadStore` at `frontend/src/stores/ticketUnread.ts` with the following surface:

```
state:
  unreadCount: number                // count of tickets with unread activity for the caller
  notificationsUnreadCount: number   // count of unread notification records
  notifications: TicketNotification[] // paginated list buffer
  notificationsPage: number
  notificationsTotal: number
  lastFetchedAt: number              // ms epoch of last successful unread-count fetch

actions:
  fetchUnreadCount(): resolves after GET .../unread-count, updates state
  fetchNotifications({page,pageSize}): loads a page into state.notifications
  markNotificationRead(id): optimistic; on failure, revert and re-fetch
  markAllNotificationsRead(): optimistic; on failure, re-fetch
  startPolling(): registers 60s interval + visibilitychange listener; idempotent
  stopPolling(): unregisters; called on user logout
  reset(): clears all state (called on logout or user switch)
```

The store SHALL detect the caller's role from `useAuthStore` and route API calls to `/api/v1/support/tickets/*` (user) vs `/api/admin/support/tickets/*` (admin) accordingly. If the caller has both `admin` role and is viewing user routes (rare edge case: admin using user-facing UI), the store SHALL follow the current route prefix.

The store SHALL be **feature-flag gated** by `support_ticket_enabled`: when the flag is `false`, `startPolling` SHALL be a no-op and all fetch actions SHALL short-circuit to no-op (avoids hitting 404 endpoints).

#### Scenario: Polling short-circuits when feature is disabled

- **GIVEN** `support_ticket_enabled = false` in public settings
- **WHEN** `startPolling()` is called
- **THEN** no HTTP requests are issued and no timers are registered

#### Scenario: Logout resets store

- **GIVEN** the store has `unreadCount = 3` and a running poll timer
- **WHEN** the user logs out (auth store transitions to unauthenticated)
- **THEN** `unreadCount = 0`, `notifications = []`, and the poll timer is cleared

#### Scenario: Optimistic markRead reverts on failure

- **GIVEN** a notification with `is_read = false` in state
- **WHEN** `markNotificationRead(id)` is invoked and the network call fails
- **THEN** the notification's `is_read` reverts to `false` in state and a warning is logged

### Requirement: Ticket Notification I18n

The frontend SHALL ship zh + en translations for the following new i18n keys (grouped under `support.ticket.notifications` and `announcementBell`):

- `announcementBell.tabs.announcement` — Announcement tab label
- `announcementBell.tabs.ticket` — Ticket tab label
- `announcementBell.actions.markAllRead` — "Mark all as read" button
- `support.ticket.notifications.empty` — Empty state text for ticket tab
- `support.ticket.notifications.event.ticketCreated` — Item label prefix for `ticket_created`
- `support.ticket.notifications.event.adminReplied` — Item label prefix for `admin_replied`
- `support.ticket.notifications.event.userReplied` — Item label prefix for `user_replied`
- `support.ticket.notifications.actorSystem` — Fallback actor name when `actor_user_id` is null
- `nav.mySupport.redDotAria` — Aria label for the user sidebar red dot
- `admin.support.tickets.redDotAria` — Aria label for the admin sidebar red dot

Backend SHALL ship zh + en templates for `support_ticket.new_ticket` and `support_ticket.new_reply` in `backend/internal/service/email_templates/` (or the existing location matching current NotificationEmailService template layout).

#### Scenario: Missing locale falls back to en

- **GIVEN** the user's locale is `ja` (Japanese) — not covered by this change
- **WHEN** an email is rendered for a ticket event
- **THEN** the email body uses the `en` template (matching existing NotificationEmailService fallback)

#### Scenario: All new keys have both zh and en entries

- **GIVEN** the frontend i18n resources are loaded
- **WHEN** any of the ten new i18n keys is looked up in either `zh` or `en`
- **THEN** the lookup returns a non-empty string (never falls through to the key itself)
