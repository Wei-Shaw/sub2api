## Why

平台目前没有任何站内的"用户反馈/问题报告"通道——遇到充值失败、API key 异常、扣费疑问等问题，用户只能去 footer 那条 fine-print 里找 QQ/QQ 群/Telegram，且这些通道是单向社群，运营无法追踪一条问题的进度、状态、解决记录。这次新增一个最小可用的站内工单系统，用户可在登录态下提交结构化问题，admin 后台能列表、回复、关闭，整个生命周期沉淀到数据库里可追溯。

这个 change 是 **客服三件套**（工单系统 / 浮窗 / RAG 知识库）里**第一个**落地的，刻意做成完全独立——不依赖任何 AI/embedding/向量库基建，单独发一版就能解决"用户找不到地方反馈"这个核心痛点。后续的浮窗和 RAG change 在它之上叠加。

## What Changes

- **新增** `support_tickets` 与 `support_ticket_replies` 两张表（PostgreSQL，通过 ent schema 管理）。
  - `support_tickets`: `id / user_id / title / content / category / status / priority / chat_context / created_at / updated_at / closed_at`。
  - `support_ticket_replies`: `id / ticket_id / author_id / is_admin / content / created_at`。
  - 索引：`(user_id, status, created_at DESC)`、`(status, priority, created_at DESC)`、`ticket_id`。
- **新增**用户端 API（必须登录）：
  - `POST /api/v1/support/tickets` 创建工单（接受 `chat_context` 参数，将来由浮窗带过来）。
  - `GET /api/v1/support/tickets` 列出当前用户的工单（分页，按 `created_at DESC`）。
  - `GET /api/v1/support/tickets/:id` 详情 + 回复列表（仅本人）。
  - `POST /api/v1/support/tickets/:id/replies` 用户追加回复（仅本人，已关闭工单不可追加）。
  - `POST /api/v1/support/tickets/:id/close` 用户主动关闭。
- **新增** admin 端 API：
  - `GET /api/admin/support/tickets` 列表，支持按 `status / category / priority / user_id / keyword` 过滤。
  - `GET /api/admin/support/tickets/:id` 详情 + 回复列表（任意工单）。
  - `POST /api/admin/support/tickets/:id/replies` admin 回复（自动 `is_admin = true`）。
  - `PATCH /api/admin/support/tickets/:id` 改 `status / priority / category`。
- **新增** admin 端配置（system_settings 字段，归到现有 `admin/settings` 页 `general` tab 下的"客服与工单"分组）：
  - `support_ticket_enabled` (bool)：总开关，关闭则用户端 API 与入口隐藏。
  - `support_ticket_categories` (string[])：工单分类列表，默认 `["充值", "账号", "API", "Bug", "其他"]`。
  - `support_ticket_default_priority` (enum: `low/normal/high`)：默认 `normal`。
- **新增**前端用户端页面：
  - `/support/tickets` 列表（带"新建工单"按钮）。
  - `/support/tickets/new` 新建表单（标题、分类、内容 Markdown 输入；支持从 URL query 接 `from=chat&context=<localStorage-key>`，自动把对话历史填充到正文草稿）。
  - `/support/tickets/:id` 详情，含历史回复时间线、追加回复输入框、"关闭工单"按钮。
- **新增**前端 admin 端页面：
  - admin 后台一级菜单"工单"（`/admin/support/tickets`）：列表 + 过滤；点击进入详情可回复、改状态、改优先级。
- **公开设置注入**：`support_ticket_enabled` 字段加入 `PublicSettings` 与 `PublicSettingsInjectionPayload`，前端 `appStore.cachedPublicSettings.support_ticket_enabled` 读取后控制入口可见性。
- **i18n**：`zh / en` 新增 `support.tickets.*`、`admin.settings.support.*`、`admin.tickets.*` 三组键。

## Capabilities

### New Capabilities

- `support-ticket`: 用户端工单的提交、查看、回复、关闭，以及 admin 端的列表、回复、状态/优先级变更。涵盖工单生命周期（`open → in_progress → closed`）、权限隔离（用户只能见自己的工单，admin 可见全部）、空状态语义（未启用 / 无工单）。

### Modified Capabilities

（无）

## Impact

- **后端**：
  - 新增 `internal/ent/schema/support_ticket.go` 与 `support_ticket_reply.go`，并跑 ent code-gen。
  - 新增 `internal/repository/support_ticket_repo.go`、`internal/service/support_ticket_service.go`。
  - 新增 `internal/handler/support_ticket_handler.go`（用户端）与 `admin_support_ticket_handler.go`（admin 端）。
  - 路由：`internal/server/routes/support.go`（新文件）注册用户端 `/api/v1/support/*` 与 admin 端 `/api/admin/support/*`。
  - `dto.PublicSettings` 与 `service.PublicSettingsInjectionPayload` 增加 `SupportTicketEnabled bool` 字段；`public_settings_injection_schema_test.go` 自动覆盖防漂移。
  - `setting_service` / `settings_view` 暴露三项新配置的 get/set。
- **数据库迁移**：
  - 两张新表的 `CREATE TABLE`；
  - 现有 `users` 表无变化，`user_id` 通过外键引用。
- **前端**：
  - 新增 `frontend/src/views/support/SupportTicketsListView.vue`、`SupportTicketNewView.vue`、`SupportTicketDetailView.vue`。
  - 新增 `frontend/src/views/admin/AdminSupportTicketsView.vue`（列表 + 详情 drawer）。
  - 新增 `frontend/src/api/support.ts`（用户端 + admin 端两组方法）。
  - `frontend/src/router/index.ts` 注册 4 条新路由（其中用户端 3 条 `requiresAuth: true`，admin 端 1 条 `requiresAdmin: true`）。
  - `frontend/src/components/layout/AppSidebar.vue` 在用户导航与 admin 导航各增加一个入口（受 `support_ticket_enabled` 守卫）。
  - `frontend/src/views/admin/SettingsView.vue` 在 `general` tab 增加"客服与工单"分组的 3 个表单项。
- **不影响**：
  - 现有 footer `HomeContactSection`（QQ/QQ 群/Telegram）不变；
  - 现有支付 / 充值 / API key / 模型计费 等任何流程；
  - 现有 admin 一级菜单顺序保持，工单菜单插在"用量"之前。
- **测试**：
  - 后端：service 与 handler 单测覆盖（创建/列表/详情/回复/关闭/权限隔离/未启用时拒绝）。
  - 前端：列表/详情/新建/admin 列表的 4 个组件 spec，至少覆盖空态、有数据、错误态、`from=chat` query 自动填充。
- **后续依赖**：
  - `add-support-chat-widget` 会读 `support_ticket_enabled` 决定是否显示"提交工单"按钮，并通过 URL query 把 localStorage 中的对话历史 key 传给 `/support/tickets/new`。
