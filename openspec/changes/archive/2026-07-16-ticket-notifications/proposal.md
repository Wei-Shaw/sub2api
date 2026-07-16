## Why

现有工单模块（`support-ticket`）在用户提交工单或管理员回复工单后，缺少任何主动通知渠道：管理员必须打开工单列表页才能发现新工单，用户回到站点也没有直观入口感知到"我有新回复"。这导致工单响应时延不可控、用户体验割裂，也让"工单"这一严肃支持通道的价值大打折扣。

本次改造要求：一旦有新工单或新回复，收件方（管理员群体 / 目标用户）必须能通过**邮件、Sidebar 菜单红点、顶部铃铛**三条渠道同时感知，且能一键跳转到具体工单。

## What Changes

- **BREAKING**（内部数据契约）：`support_tickets` 相关能力新增"未读位"语义 —— 引入 `support_ticket_reads` 表记录每个用户对每张工单的最后已读时刻。
- 新增 `support_ticket_notification` 表作为工单通知的独立事件流；每次"新工单"/"新回复"生成一条投递给对应收件人（admin 群 / ticket 所属用户）的通知记录，用于顶部铃铛"工单 tab"展示。
- 新增两个 `NotificationEmailService` 事件：`support_ticket.new_ticket`（发给管理员）与 `support_ticket.new_reply`（管理员回复时发给用户，用户回复时发给管理员），复用现有 i18n 模板机制。
- 新增全局设置项 `ticket_notify_emails`（后台管理端可配置）：非空时替代默认"全体 admin"作为管理员邮件收件人。
- 顶部 `AnnouncementBell` 组件从"单列表"改造为"多 tab"（公告 / 工单）；默认打开有未读的 tab，都有未读时默认公告 tab。
- 用户侧和管理员侧的"工单"菜单项引入服务端未读数驱动的红点（新 pinia store `useTicketUnreadStore` + 轮询）。
- 新增 REST 接口：
  - `GET /api/v1/support/tickets/unread-count`（用户端未读工单数）
  - `GET /api/v1/support/tickets/notifications`（用户端工单通知列表 + 分页）
  - `POST /api/v1/support/tickets/notifications/:id/read`（标记单条已读）
  - `POST /api/v1/support/tickets/notifications/read-all`（全部标记已读）
  - 管理员端 `GET /api/v1/admin/support/tickets/unread-count`、`GET /api/v1/admin/support/tickets/notifications`、`POST .../read`、`POST .../read-all` 四个对称接口
- 用户/管理员打开工单详情时，服务端自动写入 `support_ticket_reads.last_read_at`，红点/未读数据随之下降。

## Capabilities

### New Capabilities

- `ticket-notifications`: 工单通知能力总集 —— 定义未读位、通知记录、邮件事件、铃铛工单 tab、菜单红点及其触发/清除时机。

### Modified Capabilities

- `support-ticket`: 工单创建 / 回复流程新增 "生成通知记录 + 触发邮件 + 更新未读位" 副作用；工单详情读取端点新增"标记已读"副作用。

## Impact

- **数据库**：新增 2 张表（`support_ticket_reads`、`support_ticket_notification`）与 1 项全局设置。需 1 个 ent schema 变更 + 生成的 migration。
- **后端服务**：`support_ticket_service` 三个入口方法（Create/AppendUserReply/AppendAdminReply）增加副作用；新增 `ticket_notification_service`；`NotificationEmailService` 事件字典扩展 2 项 + 官方 zh/en 模板。
- **前端**：`AnnouncementBell.vue` 大改（tab 化）；新增 `useTicketUnreadStore`；用户侧 `AppSidebar`、管理员侧 `AdminSidebar` 菜单项接入红点；`frontend/src/api/support.ts` 新增 API 方法。
- **i18n**：新增铃铛 tab 标签、通知条目文案、邮件模板占位、菜单红点 aria-label（zh + en）。
- **权限/接口**：admin 端新增 4 个接口需 `admin` 中间件；user 端新增 4 个接口需 `auth` 中间件。
- **性能**：未读数查询走 `support_ticket_reads` 索引 + 通知表 `(recipient_user_id, is_read, created_at)` 复合索引；铃铛/红点采用 60~90s 轮询，不引入 SSE/WebSocket。
- **既有功能**：`AnnouncementBell` 的既有"公告"行为保留（现在只是变成一个 tab），API 不变；工单其它现有逻辑保持向后兼容，`support_ticket_reads` 缺失时默认视为"从未读过"。
