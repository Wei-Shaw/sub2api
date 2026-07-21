## REMOVED Requirements

### Requirement: Ticket Notification Records

**Reason**: 工单通知记录能力迁移至通用信箱模块（`general-inbox` capability）。原 `support_ticket_notification` 表继续保留一版观察期以便回滚，但从本变更起 MUST 不再写入新记录；所有新增工单事件通过 `inbox.Publisher.PublishToUser` / `PublishBroadcast` 以 `namespace="support_ticket"` 发布到 `direct_messages` 或 `broadcasts` 表。

**Migration**: 前端切换到 `useInboxStore` 后，`support_ticket_notification` 历史记录不再暴露给用户；旧记录**不 backfill 到新信箱**（`user_inbox_state.acked_seq` 懒初始化到升级时刻的 seq，历史消息全部在其之下）；旧记录随保留期结束自然作废，v2 release 决定是否 drop 表。用户仍能通过工单列表红点（`support_ticket_reads` 保留）感知历史未读工单。

### Requirement: Ticket Unread REST API (User)

**Reason**: 通知类端点 `/api/v1/support/tickets/notifications` 系列（列表、单条已读、全部已读）由通用信箱端点 `/api/v1/inbox/*` 替代。工单红点相关端点（`/api/v1/support/tickets/unread-count`）保留，见下方 MODIFIED Requirement。

**Migration**: 旧的四个 notifications 端点 MUST 保留一版兼容期，返回 `410 Gone` + response header `X-Deprecated-Use: /api/v1/inbox/messages`；下一次 release 删除。

### Requirement: Ticket Unread REST API (Admin)

**Reason**: 同上，管理端 notifications 相关端点由通用信箱端点替代；admin 的工单未读红点（`/api/admin/support/tickets/unread-count`）保留，见下方 MODIFIED Requirement。

**Migration**: 旧的四个 admin notifications 端点 MUST 保留一版兼容期返回 `410 Gone`；下一次 release 删除。

## MODIFIED Requirements

### Requirement: Ticket Notification Email Events

The system SHALL register two new notification email event types in `NotificationEmailService`:

- `support_ticket.new_ticket` — sent to admin recipients when a new ticket is created. Variables: `TicketID`, `Title`, `Excerpt`, `ActorName` (the submitter's display name or email), `PortalURL` (absolute URL to `/admin/support/tickets/:id`).
- `support_ticket.new_reply` — sent when a reply is posted. Recipient depends on direction: admin reply → ticket owner; user reply → admin recipients. Variables: `TicketID`, `Title`, `Excerpt`, `ActorName`, `IsAdminReply` (bool for template branching), `PortalURL` (owner path `/support/tickets/:id` or admin path `/admin/support/tickets/:id`).

Both events SHALL ship with official HTML + plaintext templates in `zh` and `en`. Other locales SHALL fall back to `en` (matching existing NotificationEmailService behavior).

Each `Send` invocation SHALL use `SourceType = "support_ticket"`, `SourceID = strconv.FormatInt(ticket.id, 10)`, and:
- `ReminderKey = "ticket_created"` for the new-ticket event,
- `ReminderKey = "admin_replied|<reply.id>"` or `"user_replied|<reply.id>"` for reply events.

Email dispatch failures SHALL NOT roll back the underlying ticket / reply operation (log warning only).

**在本次变更中，email 路径与通用信箱 publish 是并列的两条独立通道**：

- 工单业务代码 MUST 在同一业务流程中先调用 `inbox.Publisher.PublishToUser` / `PublishBroadcast` 发布信箱消息（实时应用内通知），再走既有 `NotificationEmailService.Send` 发送邮件。
- 信箱 publish 失败 MUST NOT 阻塞邮件发送；邮件发送失败 MUST NOT 阻塞信箱 publish。两条通道各自 fail-open（记 warning）。
- 邮件通道保持独立幂等（`ReminderKey` 唯一）；信箱通道通过 `dedup_key` 独立幂等。两条通道之间 MUST NOT 共享幂等状态。

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

#### Scenario: 信箱发布失败不阻塞邮件

- **GIVEN** 工单 T 创建流程中 `inbox.Publisher.PublishBroadcast` 因 Redis 不可用返回错误
- **WHEN** 工单创建流程继续
- **THEN** 邮件通道 MUST 正常执行 `NotificationEmailService.Send`；工单创建 API 返回 `201 Created`；日志中 MUST 记录 `warn: inbox publish failed`

#### Scenario: 邮件发送失败不阻塞信箱

- **GIVEN** 工单 T 有新的管理员回复 R，`inbox.Publisher.PublishToUser` 成功
- **WHEN** `NotificationEmailService.Send` 因 SMTP 不可达失败
- **THEN** 工单 owner 的信箱 MUST 已经收到 push；回复 API 返回 `201 Created`；日志中 MUST 记录 `warn: email send failed`

### Requirement: Sidebar Ticket Red Dot

The user-facing sidebar (`AppSidebar.vue`) SHALL render a red dot next to the "工单 / My Support" menu item when `useTicketUnreadStore().unreadCount > 0`. The admin-facing sidebar SHALL render the same red dot on the admin "工单管理 / Support Tickets" menu item using the admin-scope of the same store.

`useTicketUnreadStore().unreadCount` SHALL reflect the server's ticket-unread predicate (i.e. `GET .../unread-count`), refreshed by:

1. On store initialization / login (with a 3s delay to defer initial paint).
2. On every route change (`router.afterEach`), debounced to at most one refresh per 5 seconds.
3. On `visibilitychange` from `hidden` → `visible`.
4. **On any successful `POST` to reply / mark-read endpoints** (imperative refresh) — **replaces the previous 60-second background timer**.
5. **On every successful WS push event received on `namespace="support_ticket"`** (imperative refresh) — 由通用信箱 store 触发。

The red dot SHALL disappear immediately (optimistically) after the user opens a ticket detail page (once the client observes the corresponding `unread-count` refresh return a new lower value).

**本次变更移除背景 60 秒轮询定时器**：通用信箱通过 WS 实时推送已保证工单事件的及时性；WS 断线时由 `visibilitychange` 和路由切换事件覆盖恢复。

#### Scenario: Dot appears when unread > 0

- **GIVEN** the user has 2 tickets with unread admin replies
- **WHEN** the sidebar is rendered
- **THEN** the "工单" menu item shows the red dot

#### Scenario: Dot disappears after reading

- **GIVEN** the red dot is visible and the user opens the only unread ticket
- **WHEN** the ticket detail page loads (which triggers `unread-count` re-fetch)
- **THEN** the sidebar red dot is gone

#### Scenario: WS push triggers refresh

- **GIVEN** 用户当前 sidebar 上没有工单红点，通用信箱 WS 已连接
- **WHEN** admin 回复了该用户的工单，通用信箱 push 一条 `namespace="support_ticket"` 消息
- **THEN** 前端在 `handlePush` 中检测到 namespace 为工单，触发 `useTicketUnreadStore.fetchUnreadCount()`；sidebar MUST 在 1s 内显示红点

### Requirement: Ticket Unread Store (Frontend)

The frontend SHALL provide a pinia store `useTicketUnreadStore` at `frontend/src/stores/ticketUnread.ts` with the following surface:

```
state:
  unreadCount: number                // count of tickets with unread activity for the caller
  lastFetchedAt: number              // ms epoch of last successful unread-count fetch

actions:
  fetchUnreadCount(): resolves after GET .../unread-count, updates state
  reset(): clears all state (called on logout or user switch)
```

**本次变更移除**：`notificationsUnreadCount`, `notifications`, `notificationsPage`, `notificationsTotal`, `fetchNotifications`, `markNotificationRead`, `markAllNotificationsRead`, `startPolling`, `stopPolling` — 这些能力全部迁移到通用信箱 `useInboxStore`（按 `namespace='support_ticket'` 过滤消息）。

The store SHALL detect the caller's role from `useAuthStore` and route API calls to `/api/v1/support/tickets/unread-count` (user) vs `/api/admin/support/tickets/unread-count` (admin) accordingly.

The store SHALL be **feature-flag gated** by `support_ticket_enabled`: when the flag is `false`, `fetchUnreadCount` SHALL short-circuit to no-op (avoids hitting 404 endpoints).

#### Scenario: Fetch short-circuits when feature is disabled

- **GIVEN** `support_ticket_enabled = false` in public settings
- **WHEN** `fetchUnreadCount()` is called
- **THEN** no HTTP request is issued; state is unchanged

#### Scenario: Logout resets store

- **GIVEN** the store has `unreadCount = 3`
- **WHEN** the user logs out (auth store transitions to unauthenticated)
- **THEN** `unreadCount = 0`

### Requirement: Announcement Bell Tabbed Layout

The frontend `AnnouncementBell.vue` component SHALL present two tabs: **公告 (Announcements)** and **工单 (Tickets)**. The bell icon SHALL display a single unread badge whose count is the sum of:

- `announcementsStore.unreadCount` (existing）
- `inboxStore.unackedCountByNamespace("support_ticket")` — **由通用信箱 store 派生**，替代原 `ticketUnreadStore.notificationsUnreadCount`。

The default active tab when the panel opens SHALL follow this rule:

1. If announcement unread > 0 AND ticket inbox unread == 0 → announcement tab is active.
2. If ticket inbox unread > 0 AND announcement unread == 0 → ticket tab is active.
3. Otherwise (both > 0, or both == 0) → announcement tab is active.

Each tab SHALL show its own unread count as an inline badge in the tab header (e.g. `工单 (3)`), suppressed when that tab's count is `0`.

The Ticket tab SHALL render notification items from `useInboxStore.messages.filter(m => m.namespace === "support_ticket")`, ordered by `seq DESC`. Each item SHALL show:

- Icon inferred from `payload.event_type` (`ticket_created` / `admin_replied` / `user_replied`).
- `payload.title` prominently.
- `payload.excerpt` on a secondary line, single-line-clipped.
- Relative time (`created_at`, formatted via existing i18n time helpers).
- Visual "unread" state (highlighted background + bold title) when `seq > localAckSeq`.

Clicking a ticket notification item SHALL:

1. Navigate to `payload.link_user` (for regular users) or `payload.link_admin` (for admin users), based on the current user's role from `useAuthStore`.
2. Close the bell panel.
3. **由信箱累积 ack 机制处理"已读"状态**：随后触发的 `ack(seq)` 会推进 `localAckSeq`，未读高亮自动消失。**不再** 发送 `POST .../notifications/:id/read`。

The Ticket tab SHALL include a "全部标为已读 / Mark all as read" action, which calls `inboxStore.ackToMaxRendered()` (ack 到当前 `messages` 中最大的、且形成连续段末端的 `seq`)。

The Announcement tab SHALL preserve **all existing behavior** — layout, list content, mark-read semantics, and API calls (`announcements.ts` store) — with zero regression. Its logic SHOULD be extracted into a child component (e.g. `AnnouncementTabPanel.vue`) for maintainability, but this refactoring MUST NOT change any user-visible behavior or the announcement store's public API.

#### Scenario: Both tabs empty shows announcement tab

- **GIVEN** `announcementsStore.unreadCount = 0` and `inboxStore.unackedCountByNamespace("support_ticket") = 0`
- **WHEN** the user opens the bell
- **THEN** the Announcement tab is the active tab and the badge count is `0` (badge hidden)

#### Scenario: Only ticket inbox has unread opens ticket tab

- **GIVEN** `announcementsStore.unreadCount = 0` and `inboxStore.unackedCountByNamespace("support_ticket") = 5`
- **WHEN** the user opens the bell
- **THEN** the Ticket tab is active, the bell badge shows `5`, and the tab header shows `工单 (5)`

#### Scenario: Both tabs unread defaults to announcement

- **GIVEN** `announcementsStore.unreadCount = 2` and `inboxStore.unackedCountByNamespace("support_ticket") = 3`
- **WHEN** the user opens the bell
- **THEN** the Announcement tab is active and the bell badge shows `5`

#### Scenario: Clicking a ticket item navigates and triggers ack

- **GIVEN** 一个非管理员用户，工单 Tab 有一条未读消息（seq=1500）；`localAckSeq=1400`；`seenSeqs=[1500]`
- **WHEN** 用户点击该消息
- **THEN** 面板关闭，浏览器导航到 `payload.link_user`（如 `/support/tickets/42`）；随后 inbox store 的 `handlePush` / `trySchedulAck` 逻辑在 seq 连续段成立时推进 `localAckSeq` 到 1500（若 1500 与 1400 之间无未 render 的 seq）；MUST NOT 发送 `POST /api/v1/support/tickets/notifications/*` 请求

#### Scenario: Admin click routes to admin URL

- **GIVEN** an admin user with an unread ticket notification whose `payload.link_admin = "/admin/support/tickets/42"`
- **WHEN** the admin clicks the item
- **THEN** the browser navigates to `/admin/support/tickets/42`

#### Scenario: Announcement tab regression-free

- **GIVEN** an existing announcement `X` visible to all users
- **WHEN** a user opens the bell and interacts with the announcement tab (view / mark as read / dismiss)
- **THEN** all existing announcement behaviors work identically to before this change; the announcement store's public methods are unchanged

### Requirement: Ticket Notification I18n

The frontend SHALL ship zh + en translations for the following new i18n keys (grouped under `support.ticket.notifications` and `announcementBell`):

- `announcementBell.tabs.announcement` — Announcement tab label
- `announcementBell.tabs.ticket` — Ticket tab label
- `announcementBell.actions.markAllRead` — "Mark all as read" button
- `support.ticket.notifications.empty` — Empty state text for ticket tab
- `support.ticket.notifications.event.ticketCreated` — Item label prefix for `ticket_created`
- `support.ticket.notifications.event.adminReplied` — Item label prefix for `admin_replied`
- `support.ticket.notifications.event.userReplied` — Item label prefix for `user_replied`
- `support.ticket.notifications.actorSystem` — Fallback actor name when actor is empty
- `nav.mySupport.redDotAria` — Aria label for the user sidebar red dot
- `admin.support.tickets.redDotAria` — Aria label for the admin sidebar red dot

Backend SHALL ship zh + en templates for `support_ticket.new_ticket` and `support_ticket.new_reply` in the existing NotificationEmailService template layout.

**本次变更不修改 i18n 键名**：迁移到通用信箱后，事件类型仍复用 `event.ticketCreated` / `event.adminReplied` / `event.userReplied`，只是消息数据源改为从 `payload.event_type` 读取。

#### Scenario: Missing locale falls back to en

- **GIVEN** the user's locale is `ja` (Japanese) — not covered by this change
- **WHEN** an email is rendered for a ticket event
- **THEN** the email body uses the `en` template (matching existing NotificationEmailService fallback)

#### Scenario: All new keys have both zh and en entries

- **GIVEN** the frontend i18n resources are loaded
- **WHEN** any of the ten new i18n keys is looked up in either `zh` or `en`
- **THEN** the lookup returns a non-empty string (never falls through to the key itself)

## ADDED Requirements

### Requirement: 工单事件通过通用信箱发布

工单 Service SHALL 在以下事件发生时通过 `inbox.Publisher` 发布信箱消息：

- **用户创建工单** → `PublishBroadcast(namespace="support_ticket", dedupKey="created:<ticket_id>", targeting={role:"admin"}, payload={...})`
- **管理员回复工单** → `PublishToUser(namespace="support_ticket", dedupKey="admin_reply:<reply_id>", recipientID=<ticket.user_id>, payload={...})`
- **用户回复工单** → `PublishBroadcast(namespace="support_ticket", dedupKey="user_reply:<reply_id>", targeting={role:"admin"}, payload={...})`

Payload 结构 MUST 至少包含：

```json
{
  "event_type": "ticket_created" | "admin_replied" | "user_replied",
  "ticket_id": <int64>,
  "title": "<string>",              // ticket title snapshot
  "excerpt": "<string>",            // Markdown-stripped, ≤200 chars
  "actor_name": "<string|null>",
  "link_user": "/support/tickets/<id>",
  "link_admin": "/admin/support/tickets/<id>"
}
```

`excerpt` 计算规则复用原 `support_ticket_notification.excerpt` 规范（Markdown 剥离 + 空白折叠 + 200 字符截断 + `…`）。

Publish 失败 MUST 只 log warning，MUST NOT 阻塞工单创建/回复主流程。

`namespace="support_ticket"` MUST 在 `inbox.RegisterNamespace` 显式注册。

#### Scenario: 用户创建工单触发广播

- **GIVEN** 用户 U 创建了工单 T (id=42, title="充值未到账")
- **WHEN** `TicketService.CreateTicket` 成功持久化后
- **THEN** MUST 调用 `PublishBroadcast(namespace="support_ticket", dedupKey="created:42", targeting={role:"admin"}, payload={event_type:"ticket_created", ticket_id:42, title:"充值未到账", ...})`

#### Scenario: 管理员回复触发单播

- **GIVEN** 工单 T 归属用户 U (id=100)，管理员 A 添加了回复 R (id=555)
- **WHEN** `TicketService.AddReply` 成功持久化后
- **THEN** MUST 调用 `PublishToUser(namespace="support_ticket", dedupKey="admin_reply:555", recipientID=100, payload={event_type:"admin_replied", ...})`

#### Scenario: 重复 publish 幂等

- **GIVEN** 工单创建流程调用 `PublishBroadcast(namespace="support_ticket", dedupKey="created:42", ...)` 已成功
- **WHEN** 因业务重试再次调用同样参数
- **THEN** 通用信箱层由 `broadcasts` 表 `UNIQUE (namespace, dedup_key)` 保证只落一条；工单业务侧收到相同 seq

#### Scenario: 通用信箱不可用不阻塞工单

- **GIVEN** 通用信箱 `inbox_v1_enabled=false` 或 Redis 不可用
- **WHEN** 用户提交工单
- **THEN** publish 静默返回错误（或 no-op）；工单 API MUST 返回 `201 Created`；邮件通道正常执行

### Requirement: 旧通知端点返回 410 Gone

以下 REST 端点在本次变更后 MUST 返回 `410 Gone`，并在 response header 中携带 `X-Deprecated-Use: /api/v1/inbox/messages`：

- `GET /api/v1/support/tickets/notifications`
- `POST /api/v1/support/tickets/notifications/:id/read`
- `POST /api/v1/support/tickets/notifications/read-all`
- `GET /api/admin/support/tickets/notifications`
- `POST /api/admin/support/tickets/notifications/:id/read`
- `POST /api/admin/support/tickets/notifications/read-all`

保留的 unread-count 端点 MUST 继续按原逻辑工作：

- `GET /api/v1/support/tickets/unread-count`（用户工单红点）
- `GET /api/admin/support/tickets/unread-count`（管理员工单红点）

#### Scenario: 旧端点返回 410

- **WHEN** 客户端调用 `GET /api/v1/support/tickets/notifications?page=1`
- **THEN** 响应 status MUST 为 `410`，body MUST 为 `{"error":"deprecated","use":"/api/v1/inbox/messages"}`，response header MUST 包含 `X-Deprecated-Use: /api/v1/inbox/messages`

#### Scenario: unread-count 端点保留

- **GIVEN** 用户 U 有 3 张有未读活动的工单
- **WHEN** U 调用 `GET /api/v1/support/tickets/unread-count`
- **THEN** MUST 返回 `200 {"count": 3}`（行为不变）

### Requirement: 不 backfill 历史工单通知

系统 MUST NOT 在升级过程中将存量 `support_ticket_notification` 记录批量导入 `direct_messages` 或 `broadcasts`。

产品语义 MUST 定为"通用信箱从上线时刻启用"：老用户升级后首次开启信箱是空的（`user_inbox_state.acked_seq` 懒初始化到升级时刻的 seq，所有历史消息 seq 均在其之下）。

用户 MUST 仍可通过工单列表红点（基于 `support_ticket_reads`）感知升级前的未读工单，红点行为由现有 `Ticket Read State Tracking` Requirement 保证不变。

`support_ticket_notification` 表 MUST 保留一版观察期不写入新记录；下一次 release 决定是否 drop。

#### Scenario: 老用户升级后首次开信箱是空的

- **GIVEN** 用户 U 在升级前 `support_ticket_notification` 有 5 条未读记录
- **WHEN** 升级完成后 U 首次调用 `GET /api/v1/inbox/messages?since_seq=0`
- **THEN** 服务端懒初始化 `user_inbox_state.acked_seq = fresh_seq(now)`；`messages` MUST 返回空数组

#### Scenario: 升级不写入 backfill 记录

- **GIVEN** 升级前 `direct_messages`, `broadcasts` 表为空
- **WHEN** 升级迁移 SQL 执行完成
- **THEN** 两张表 MUST 仍为空；MUST NOT 从 `support_ticket_notification` 复制数据

#### Scenario: 工单红点仍显示历史未读

- **GIVEN** 用户 U 在升级前有一张工单 T，管理员回复了但 U 未读
- **WHEN** U 升级后打开界面
- **THEN** 侧栏"工单"红点 MUST 仍显示（因为 `support_ticket_reads` 未变化）；U 打开 T 后 `support_ticket_reads.last_read_at` 更新，红点消失
