## Context

sub2api 现有工单系统 (`internal/service/support_ticket_service.go`) 已经完整支持"用户提工单 / 管理员回复 / 状态机 open→in_progress→closed / 附件"等能力，但只提供**同步查询接口**：任意一方要发现新动作，都必须主动打开工单列表页。

生态里已经有的可复用组件：

- `notification_email_service.go`：14 类业务邮件的统一入口，提供事件常量 + `NotificationEmailEventInfo` 定义 + i18n 模板 + `deliveryKey` 幂等 + 队列投递。加事件是"添加常量 + 定义 + 模板 zh/en"三点工作量。
- `AnnouncementBell.vue`：顶部公告铃铛（当前是单列表结构，无 tab）。使用 `announcements.ts` pinia store 拉未读广播公告。
- `AppSidebar.vue` / 管理员侧 sidebar：`NavItem.showDot?: () => boolean` 契约已定义并在多个菜单项落地（`flagPurchasePromoDot`、`flagCustomMenuDotForItem`），但这些红点数据源都是本地 flag / localStorage dismiss，**没有"服务端未读数驱动"的先例**。

不可复用 / 需要新造：
- 工单未读位（`support_tickets` 与 `support_ticket_replies` 目前没有任何 read/unread 字段）。
- "定向到某个 recipient 的通知流"（`announcement` 是广播语义，`AnnouncementRead` 只做"读时过滤"，塞不下"给单个用户投递一条通知")。

约束：
- 沿用现有 ent + pgx 数据栈；DB migration 走仓库既有 `atlas` 流程（`ent generate` + 手写 SQL migration）。
- 前端沿用 vue3 + pinia + i18n（zh/en）栈，禁止引入新的 WebSocket / SSE 依赖。
- 邮件必须走 `NotificationEmailService.Send`，不允许绕过（否则丢队列 / 幂等 / i18n 三个能力）。

## Goals / Non-Goals

**Goals:**

- 新工单 / 新回复触发后，**收件方在同一次动作里同时得到三个感知**：邮件（异步队列）、Sidebar 红点、顶部铃铛工单 tab 条目。
- 未读位存储粒度到**每个 user × ticket**，管理员之间互不干扰（决策：方案 B）。
- 通知记录采用**独立事件表**（决策：选项 X），两端共用一张 `support_ticket_notification`，靠 `recipient_user_id` 区分。
- 管理员邮件收件人**支持后台配置**，未配置时兜底为"所有 role=admin 用户"（决策：方案 γ）。
- 铃铛 tab 默认打开策略：**优先未读那个 tab；两个都有未读，默认公告；两个都没未读，默认公告**。
- 60~90s 轮询即可满足实时性要求；不做 WebSocket。
- 邮件不做节流 / 合并 —— 每次事件独立发送。

**Non-Goals:**

- 工单接收范围的 **per-ticket assign**：本次不做（决策：全局，所有工单可以被任一 admin 看到）。
- 通知合并 / 摘要邮件（每 5min 合成一封）：不做。
- 桌面通知 / PWA push：不做。
- 通知条目的富交互（点赞 / @ / 引用）：不做，纯"标题 + 摘要 + 跳转"。

## Decisions

### 决策 1：未读用独立表 `support_ticket_reads`（方案 B）

**选择**：新表 `support_ticket_reads(id, ticket_id, user_id, last_read_at, created_at, updated_at)`，唯一索引 `(ticket_id, user_id)`。

**替代方案**：
- A（在 `support_tickets` 加 `user_last_read_at` / `admin_last_read_at`）：管理员之间未读共享，A 管理员点开后 B 也不红了。
- C（复用 announcement 系统）：广播语义不匹配，会污染公告数据。

**为什么选 B**：管理员未来可能多人协作，A 语义有信息损失；B 是 A 的严格超集，字段冗余可忽略，聚合成本用复合索引就能压平。

**未读判定**：
- 用户对工单 `t` **有未读** ⇔ 存在 `t.replies` 中 `is_admin=true` 且 `created_at > coalesce(reads.last_read_at, '1970-01-01')`。
- 管理员对工单 `t` **有未读** ⇔ (工单本身 `t.created_at > coalesce(reads.last_read_at, '1970-01-01')`) OR (存在 `t.replies` 中 `is_admin=false` 且 `created_at > coalesce(...)`）。
- `support_ticket_reads` 是 lazy-created：首次调用"标记已读"时 upsert。

**索引**：`(user_id, ticket_id)` 唯一 + `(user_id, last_read_at)` 用于未读数聚合。

### 决策 2：通知走独立表 `support_ticket_notification`（选项 X）

**选择**：新表 —— 每次触发事件，向每个 recipient 写一行。

```
support_ticket_notification
─────────────────────────────
id                bigserial PK
recipient_user_id int64      -- FK users.id，索引
ticket_id         int64      -- FK support_tickets.id，索引
event_type        string(32) -- "ticket_created" | "admin_replied" | "user_replied"
title_snapshot    string(200)-- 冗余工单标题（工单标题被改也不影响历史通知）
excerpt           string(500)-- 首次提交/回复正文的截断（Markdown 剥离 + 首 200 字）
actor_user_id     int64 nullable -- 触发者（用户提交时=user；admin 回复时=admin）
is_read           bool default false
created_at        timestamptz
read_at           timestamptz nullable
```

**索引**：`(recipient_user_id, is_read, created_at desc)` 单一复合索引即可服务"未读计数""通知列表分页""未读中最新一条"三个查询。

**替代方案**：
- Y（"未读工单本身"当通知条目）：一开始更倾向这个，但你选了 X。X 的优势是允许"同一工单在通知列表里出现多条历史"，跟"公告列表 = 广播历史"的形态一致，UI 更规整。

**权衡（X 的代价）**：
- 写扩散：一次事件 × N 个 admin = N 行。假设 admin 数 ≤ 20，忽略。
- 通知与工单可能语义漂移：例如工单被删除，通知记录不联动删除（改为查询时用 LEFT JOIN 过滤，或删除工单时级联删 notification —— 我选 **`ON DELETE CASCADE`**，保持"孤儿通知不存在"的强一致）。

### 决策 3：邮件事件与收件人策略（决策 γ + 事件常量）

**新增两个事件常量**（放在 `notification_email_service.go`）：

```go
NotificationEmailEventSupportTicketNewTicket = "support_ticket.new_ticket"
NotificationEmailEventSupportTicketNewReply  = "support_ticket.new_reply"
```

**触发规则**：
- 用户创建工单 → `new_ticket` 发给"管理员群"。
- 用户回复工单 → `new_reply` 发给"管理员群"。
- 管理员回复工单 → `new_reply` 发给"工单所属用户"。

**"管理员群" 解析（方案 γ）**：
1. 读取全局设置 `ticket_notify_emails`（新增设置项，`AdminSettings` 里挂）—— 字符串数组，非空即使用。
2. 空则查 `users` 表所有 `role = 'admin'` 的 email 汇总。

**幂等**：走 `NotificationEmailInput.SourceType = "support_ticket"` + `SourceID = ticket_id` + `ReminderKey = event_type|reply_id` 组合，天然避免同一事件重复送。

**模板**（走既有官方模板机制）：
- `templates/notifications/support_ticket/new_ticket_zh.tmpl` + `_en.tmpl`
- `templates/notifications/support_ticket/new_reply_zh.tmpl` + `_en.tmpl`

变量：`{{.TicketID}}`、`{{.Title}}`、`{{.Excerpt}}`、`{{.ActorName}}`、`{{.PortalURL}}`（跳工单详情页的完整 URL，管理员链接指向 `/admin/support/tickets/:id`，用户链接指向 `/support/tickets/:id`）。

### 决策 4：副作用如何挂到 service（同步 or 异步）

**选择**：**同步 in-process**，在 `CreateTicket` / `AppendUserReply` / `AppendAdminReply` 成功返回前，串联调用：

```go
notifyTicketEvent(ctx, ticket, replyOrNil, event) {
    // 1. 写 support_ticket_notification (N 行)
    // 2. 调 NotificationEmailService.Send（本身走队列，异步）
}
```

**为什么同步**：
- `NotificationEmailService.Send` 内部已经把"投递"做成异步（塞进队列），主流程只承担"入库通知记录"的开销 —— 一次批量 INSERT，成本可忽略。
- 简化事务边界：通知记录写失败可以 `logger.Warn` 降级（业务不阻塞），不需要引入 outbox / retry queue。

**错误策略**：
- 邮件发送失败：`log warn`，不回滚工单创建 / 回复（现有 email service 语义也如此）。
- 通知记录写失败：同样 `log warn`。
- 用一个 helper `ticket_notification_service.go` 封装，隔离 `support_ticket_service.go` 主逻辑。

### 决策 5：铃铛 tab 改造 —— 就地改 `AnnouncementBell.vue`

**选择**：直接把现在的 `AnnouncementBell.vue` 从"单列表"改为"tab 容器"，不新开组件。理由：
- 顶部只保留一个铃铛图标 / 一个 badge（总未读数），减少视觉噪音。
- tab 内部 = 独立组件（`AnnouncementTabPanel.vue` 抽出原公告逻辑 + 新建 `TicketNotificationTabPanel.vue`），保持职责单一。

**默认 tab 打开策略**（都在 `AnnouncementBell.vue` 里派生）：

```
公告未读数 = announcementsStore.unreadCount
工单未读数 = ticketUnreadStore.notificationsUnreadCount

def defaultTab():
    if 公告未读 > 0 and 工单未读 == 0: return 'announcement'
    if 工单未读 > 0 and 公告未读 == 0: return 'ticket'
    if 公告未读 > 0 and 工单未读 > 0: return 'announcement'  # 用户偏好
    return 'announcement'
```

badge 数 = 公告未读 + 工单未读。

### 决策 6：Sidebar 红点 —— 新 pinia store 驱动

**新增** `frontend/src/stores/ticketUnread.ts`，抽象两端共用（按当前登录角色决定调用哪个端点）：

```ts
useTicketUnreadStore = {
  state: { unreadCount: 0, notificationsUnreadCount: 0, lastFetchedAt: 0 },
  actions: {
    fetchUnread(),          // 视角度调 /api/v1/support/tickets/unread-count 或 /admin
    fetchNotifications({page,pageSize}),
    markNotificationRead(id),
    markAllNotificationsRead(),
    markTicketRead(ticketId),
    startPolling()          // 60s 间隔 + visibilitychange
  }
}
```

**未读数与通知未读数分离**（`unreadCount` = 有未读回复的工单数；`notificationsUnreadCount` = 未读通知条目数）：
- Sidebar 红点用 `unreadCount > 0`（更贴合"有事情待处理的工单数"直觉）。
- 铃铛 badge 里的工单 tab 数字用 `notificationsUnreadCount`（对齐"通知条目"）。
- 两者相互独立，避免"点开工单详情已清工单未读，但铃铛里的通知条目仍在"这种"打脸"的错觉。

**AppSidebar 挂法**：

```ts
{ path: '/support/tickets', label: t('nav.mySupport'), icon: TicketIcon,
  showDot: () => useTicketUnreadStore().unreadCount > 0 }
```

管理员侧对称接入。

### 决策 7：接口路由 & 中间件

| 方法 | 路径 | 中间件 | 说明 |
|---|---|---|---|
| GET | `/api/v1/support/tickets/unread-count` | `auth` | 返回当前用户"有未读回复的工单数" |
| GET | `/api/v1/support/tickets/notifications` | `auth` | 分页返回当前用户的工单通知列表 |
| POST | `/api/v1/support/tickets/notifications/:id/read` | `auth` | 标记单条已读，鉴权检查 recipient_user_id == 当前用户 |
| POST | `/api/v1/support/tickets/notifications/read-all` | `auth` | 全部标记已读 |
| GET | `/api/v1/admin/support/tickets/unread-count` | `admin` | 管理员未读工单数 |
| GET | `/api/v1/admin/support/tickets/notifications` | `admin` | 管理员工单通知列表 |
| POST | `/api/v1/admin/support/tickets/notifications/:id/read` | `admin` | 标记已读 |
| POST | `/api/v1/admin/support/tickets/notifications/read-all` | `admin` | 全部标记已读 |

**已读的隐式触发**：现有的 `GET /support/tickets/:id`（用户端）与 `GET /admin/support/tickets/:id`（管理员端）在返回工单详情前 upsert `support_ticket_reads.last_read_at = now()`（对当前登录用户）。这样"用户/管理员打开工单详情"自动清工单红点，不需要前端显式调 markRead。

## Risks / Trade-offs

- **[风险] 邮件轰炸**：管理员多、工单多时可能被邮件淹没 → **缓解**：`ticket_notify_emails` 白名单允许"只发运维值班组"；且短期同一 ticket 内的多次回复通过 `ReminderKey=event_type|reply_id` 保持逐条送达（用户偏好），不做合并 —— 若后续投诉，加"5min 内合并"作为增量能力。
- **[风险] 未读位漂移**：管理员打开工单后 `admin_last_read_at` 被写入，但正好这一瞬间又来了新回复 → **缓解**：写读时刻用 `now()` 严格 = 请求处理时刻，而不是"读取到的最后一条回复的时间"，保证"处理完之后再来的回复必然被识别为未读"。
- **[风险] 通知孤儿**：工单被硬删除 → **缓解**：`support_ticket_notification.ticket_id` 外键 `ON DELETE CASCADE`。
- **[风险] 前端 tab 改造带来的既有公告回归**：`AnnouncementBell` 是核心组件 → **缓解**：先把现有逻辑抽到 `AnnouncementTabPanel.vue` 保持 API 完全一致，再改外壳；在 `AnnouncementBell.spec.ts` 补充"公告 tab 默认打开、未读数聚合正确"两个测试。
- **[Trade-off] 轮询开销**：60s 间隔 + 2 个 store × 每用户 = 每分钟 4 次请求 → 可接受，`unread-count` 是极轻查询（走复合索引）。
- **[Trade-off] `is_read=true` 后不再展示对已读工单的通知历史**：X 方案下用户可以在铃铛列表里翻历史；配合 `read-all` 按钮避免长列表堆积。默认列表分页 20 条，滚动加载。
- **[风险] i18n 不一致**：项目里有 zh/en/ja/ko/ru 多语言，其他语言短期不覆盖 → **缓解**：只交付 zh + en（与既有邮件模板一致），其他语言回退至 en。
