# Support Ticket Notifications & Unread Count API

## Overview

支持工单系统在"新工单"、"用户回复"、"管理员回复"事件发生时会同步写入两类数据：

1. **`support_ticket_notification` 记录** — 铃铛面板 (`AnnouncementBell`) 的通知条目。每条记录只属于唯一收件人 (`recipient_user_id`)，前端可拉列表、单条标已读、批量清空。
2. **`support_ticket_reads` 读游标** — 每 `(ticket_id, user_id)` 组合的最新阅读时间。用于"未读工单数聚合"：
   - 用户视角未读工单：owner 名下有 admin 回复晚于自己的 `last_read_at`。
   - 管理员视角未读工单：工单 `created_at` 或 最新用户回复时间 晚于当前 admin 的 `last_read_at`。

本文档描述与这两类数据相关的 8 个 HTTP 端点：4 个用户视角 + 4 个 admin 视角对称。

### 通用约定

- 所有端点都要求登录（挂 `JWTAuthMiddleware`；未认证返回 `401`）。
- 请求/响应体统一走项目内 `response.Success` / `response.ErrorFrom` 信封：

  ```json
  { "code": 0, "message": "success", "data": <payload> }
  ```

- 时间字段统一 ISO 8601 UTC (`2026-07-16T10:00:00Z`)。
- 分页字段与其他 admin API 一致：`page`、`page_size`、`pages`、`total`、`items`。
- 一切写操作是**幂等**的（重复 mark-read 不会报错、`read-all` 返回 `affected=0`）。

### Feature Flag

若 admin 关闭 `support_ticket_enabled`：
- 用户端 4 个端点全部返回 `404`（后端 sidebar 入口也隐藏）。
- **admin 端 4 个端点仍可用**，方便运营继续处理存量通知/工单（与 admin 工单主 CRUD 一致的策略）。

前端 (`useTicketUnreadStore`) 会在 `support_ticket_enabled=false` 时空跑 fetch / polling，不主动清空已有 state。

---

## User Endpoints

### GET `/api/v1/support/tickets/unread-count`

获取当前用户视角的未读工单数（用于 Sidebar 红点、AnnouncementBell 徽标）。

**未读定义**：调用方 owner 的工单里存在管理员回复且 `reply.created_at > support_ticket_reads.last_read_at`。

**Request**

```http
GET /api/v1/support/tickets/unread-count
Authorization: Bearer <jwt>
```

**Response 200**

```json
{
  "code": 0,
  "message": "success",
  "data": { "count": 3 }
}
```

**Status codes**

- `200` 成功。`count` 类型 `int64`（前端可 clamp 到 `99+`）。
- `401` 未登录。
- `404` `support_ticket_enabled=false`（feature disabled）。

---

### GET `/api/v1/support/tickets/notifications`

分页拉当前用户名下的铃铛通知条目。按 `created_at DESC` 排序。

**Query**

| 字段 | 类型 | 默认 | 说明 |
|---|---|---|---|
| `page` | int | 1 | 1-based |
| `page_size` | int | 20 | 上限由 `response.ParsePagination` 决定（100） |
| `only_unread` | bool | false | `"true"` 时只返回 `is_read=false`（其他值均视为 false） |

**Request**

```http
GET /api/v1/support/tickets/notifications?page=1&page_size=20
Authorization: Bearer <jwt>
```

**Response 200**

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "items": [
      {
        "id": 101,
        "recipient_user_id": 42,
        "ticket_id": 555,
        "event_type": "admin_replied",
        "title_snapshot": "登录失败",
        "excerpt": "已收到您的工单，将在 24h 内处理",
        "actor_user_id": 7,
        "is_read": false,
        "created_at": "2026-07-16T10:00:00Z",
        "read_at": "0001-01-01T00:00:00Z"
      }
    ],
    "total": 1,
    "page": 1,
    "page_size": 20,
    "pages": 1
  }
}
```

**Item 字段说明**

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | int64 | 通知记录主键 |
| `recipient_user_id` | int64 | 通知归属用户 ID（隔离键） |
| `ticket_id` | int64 | 关联工单；前端点击跳 `/support/tickets/:id` |
| `event_type` | string | `ticket_created` / `user_replied` / `admin_replied` |
| `title_snapshot` | string | 事件发生时的工单标题快照（防止后续标题被修改导致 bell 显示错乱） |
| `excerpt` | string | 事件正文摘要（≤ 200 字符，rune 级截断） |
| `actor_user_id` | int64 | 触发者用户 ID；`0` 表示未知（作者被删除或匿名场景） |
| `is_read` | bool | 是否已读；前端根据此字段决定是否解读 `read_at` |
| `created_at` | ISO8601 | 事件时间 |
| `read_at` | ISO8601 | 已读时间；`is_read=false` 时为 Go zero time `0001-01-01T00:00:00Z`（**不要**单独依赖此字段判断已读） |

**Status codes**

- `200` 成功；空列表返回 `items: []`（不是 `null`）。
- `401` 未登录。
- `404` feature disabled。

---

### POST `/api/v1/support/tickets/notifications/:id/read`

把单条通知标为已读。

**Path**

- `id` int64 (`> 0`)：通知记录 ID。

**Request**

```http
POST /api/v1/support/tickets/notifications/101/read
Authorization: Bearer <jwt>
```

无请求体。

**Response 200**

```json
{
  "code": 0,
  "message": "success",
  "data": { "id": 101 }
}
```

**Semantics**

- **幂等**：已读记录重复调用不报错，仍返回 `200`。
- **访问控制**：`(id, recipient_user_id)` 二元定位；不属于当前用户的通知返回 `404`，不区分"不存在"与"越权"（防止 recipient 探测）。

**Status codes**

- `200` 成功。
- `400` `id` 非法（非数字 / ≤ 0）。
- `401` 未登录。
- `404` 通知不存在 / 不属于当前用户 / feature disabled。

---

### POST `/api/v1/support/tickets/notifications/read-all`

一次性把当前用户所有未读通知标已读。

**Request**

```http
POST /api/v1/support/tickets/notifications/read-all
Authorization: Bearer <jwt>
```

无请求体。

**Response 200**

```json
{
  "code": 0,
  "message": "success",
  "data": { "affected": 5 }
}
```

**Semantics**

- **幂等**：无未读时返回 `affected=0`，仍是 `200`。
- 只影响 `is_read=false` 的行；已读记录不重复覆盖 `read_at`。

**Status codes**

- `200` 成功。
- `401` 未登录。
- `404` feature disabled。

---

## Admin Endpoints

以下 4 个端点与用户端**结构完全对称**，仅差异如下：

| 差异点 | 用户端 | Admin 端 |
|---|---|---|
| URL 前缀 | `/api/v1/support/tickets` | `/api/v1/admin/support/tickets` |
| 中间件 | `JWTAuthMiddleware` | `AdminAuthMiddleware` |
| `unread-count` 聚合 | `CountUnreadForUser`：owner 有 admin 回复晚于 `last_read_at` | `CountUnreadForAdmin`：工单 `created_at` OR 最新用户回复 晚于该 admin 的 `last_read_at` |
| feature disabled 时可用 | ❌ 404 | ✅ 可用（与 admin 工单 CRUD 一致的策略） |
| Notification 记录归属 | 只能操作 `recipient_user_id=当前用户` 的记录 | 同左（admin 也只能操作**发给自己**的通知；管理员群体的每人都有独立记录） |

### GET `/api/v1/admin/support/tickets/unread-count`

`data.count`：全站"新工单 OR 有用户回复且当前 admin 未看过"的工单数量。

### GET `/api/v1/admin/support/tickets/notifications`

query / response shape 同用户端。

### POST `/api/v1/admin/support/tickets/notifications/:id/read`

### POST `/api/v1/admin/support/tickets/notifications/read-all`

均与用户端语义对称。

---

## 事件与通知记录写入触发点

以下发生在 `SupportTicketService` 主流程成功 return 之后，作为 best-effort 副作用；任何一步失败都只记 `warn` 日志，不影响主 API 的响应。

| 事件 | 触发点 | 通知记录写入方 | 记录 `event_type` | 邮件事件 |
|---|---|---|---|---|
| 用户新建工单 | `CreateTicket` | 每位管理员（settings 白名单或全体 admin） | `ticket_created` | `support_ticket.new_ticket` |
| 用户新回复 | `AppendUserReply` | 每位管理员 | `user_replied` | `support_ticket.new_reply` (`reply_kind_label` = "用户回复") |
| 管理员回复 | `AppendAdminReply` 事务提交后 | 工单 owner 一人 | `admin_replied` | `support_ticket.new_reply` (`reply_kind_label` = "客服回复") |

同时，`SupportTicketService.GetUserTicket` / `GetAdminTicket` / `AppendUserReply` / `AppendAdminReply` 会 upsert `support_ticket_reads.last_read_at`（读游标），让"打开详情"和"回复"都自动清红点。

## 管理员邮件收件白名单

Admin 系统设置 `support_ticket_notify_emails` 是一个 JSON 数组（`[]NotifyEmailEntry`），用于覆盖"新工单 / 新回复"事件的**管理员方向**邮件收件白名单：

- **非空** → 使用白名单作为邮件收件人；`disabled=true` 的项被跳过；系统会尝试根据 `email` 匹配 `role=admin` 用户，匹配到的用户既写通知记录也发邮件，匹配不到的 email 只发邮件。
- **空 / 未配置** → 兜底为所有 `role=admin` 且 `status=active` 的用户邮箱。

站内通知记录 (`support_ticket_notification`) 始终只写给系统内 `role=admin` 用户，白名单里 email-only 的收件人不写入记录。

### 归一化规则（写入前）

- Email trim + 空白项丢弃；
- 单条 `email` 长度 > `SupportTicketNotifyEmailMaxLen` (254 rune) 丢弃；
- 按小写 email 去重（保留第一次出现，含其原始大小写、`disabled` / `verified` 状态）；
- 超出 `SupportTicketNotifyEmailsMaxCount` (20 项) 截断。

Runtime 层读取 (`GetSupportTicketRuntime`) 会额外做：

- 过滤掉 `disabled=true` 项；
- Trim + 小写 + 去重后返回 `[]string`。

---

## 相关代码入口

- 后端 handler：
  - `backend/internal/handler/support_ticket_notification_handler.go`
  - `backend/internal/handler/admin/support_ticket_notification_handler.go`
- 后端 service：
  - `backend/internal/service/support_ticket_service.go`（`CountUserUnreadTickets` / `CountAdminUnreadTickets`）
  - `backend/internal/service/support_ticket_notification_service.go`（`ListNotifications` / `CountUnread` / `MarkOneRead` / `MarkAllRead` / `Notify*`）
- 后端 repository：
  - `backend/internal/repository/support_ticket_notification_repo.go`
  - `backend/internal/repository/support_ticket_read_repo.go`
- 前端 API client：`frontend/src/api/support.ts` (`getTicketUnreadCount` / `getTicketNotifications` / `markTicketNotificationRead` / `markAllTicketNotificationsRead` 及 admin 对称版本)
- 前端 store：`frontend/src/stores/ticketUnread.ts`
