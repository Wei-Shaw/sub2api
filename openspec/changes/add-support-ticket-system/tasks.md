## 1. 后端：ent schema 与迁移

- [ ] 1.1 新建 `backend/internal/ent/schema/support_ticket.go`，定义字段：`id (BigInt) / user_id (BigInt) / title (200) / content (TEXT) / category (50) / status (20, default open) / priority (20, default normal) / chat_context (TEXT, optional) / created_at / updated_at / closed_at (optional)`，并定义 edge `replies → SupportTicketReply`、`user → User`
- [ ] 1.2 新建 `backend/internal/ent/schema/support_ticket_reply.go`，定义字段：`id / ticket_id / author_id / is_admin (bool) / content (TEXT) / created_at`，edge `ticket → SupportTicket`、`author → User`
- [ ] 1.3 在 schema 上加索引：`SupportTicket` 索引 `(user_id, status, created_at)` 与 `(status, priority, created_at)`；`SupportTicketReply` 索引 `(ticket_id, created_at)`
- [ ] 1.4 跑 `go generate ./internal/ent/...` 重新生成 ent client；`go build ./...` 验证编译通过
- [ ] 1.5 在迁移机制中（auto-migrate / 手写 migration，按项目惯例）确保两张新表在启动时被创建；不需要写 down 迁移

## 2. 后端：Settings 接入

- [ ] 2.1 在 `internal/handler/dto/settings.go` 的 `PublicSettings` 结构体增加 `SupportTicketEnabled bool` 字段（带 json tag）
- [ ] 2.2 在 `internal/service/settings_view.go` 的 `PublicSettingsInjectionPayload` 同步增加该字段；通过 `public_settings_injection_schema_test.go` 自动覆盖防漂移
- [ ] 2.3 在 setting_service 内增加三项 setting 的 getter / setter：`support_ticket_enabled` (bool, default false)、`support_ticket_categories` (string[] JSON, default `["充值","账号","API","Bug","其他"]`)、`support_ticket_default_priority` (enum string, default `"normal"`)
- [ ] 2.4 setter 内做校验：`support_ticket_categories` 非空、每项 1..20 字符、最多 20 项、不重复；`default_priority` ∈ `{low, normal, high}`
- [ ] 2.5 admin settings 全量 GET / PATCH 接口（已有的）把这三个字段一并暴露；`GetPublicSettings` 把 `support_ticket_enabled` 投影到响应 DTO

## 3. 后端：Repository

- [ ] 3.1 新建 `internal/repository/support_ticket_repo.go`，方法：`Create(ctx, ticket) error / GetByID(ctx, id) (Ticket, error) / ListByUser(ctx, userID, page, pageSize) ([]Ticket, total) / ListAdmin(ctx, filter, page, pageSize) ([]Ticket, total) / UpdateFields(ctx, id, patch) error / AppendReply(ctx, reply) error / ListReplies(ctx, ticketID) ([]Reply, error)`
- [ ] 3.2 List 方法 SELECT 的字段**不**包含 `chat_context`；GetByID 才 SELECT it（防止列表查询拖大）
- [ ] 3.3 Admin 列表的 `q` 参数走 `WHERE title ILIKE '%q%' OR content ILIKE '%q%'`，参数化拼接防注入
- [ ] 3.4 Admin 列表排序写死 `priority DESC, created_at DESC`（用 `CASE` 表达式把 `low|normal|high` 映射为 `1|2|3` 再 DESC，或者在 schema 里 priority 用整型——选 string + CASE 保持 schema 简单）
- [ ] 3.5 单测 `support_ticket_repo_test.go`：覆盖创建、按 user 查、admin 过滤组合（status × priority × q）、reply append、list reply 排序、list 不返回 chat_context

## 4. 后端：Service 层（业务规则）

- [ ] 4.1 新建 `internal/service/support_ticket_service.go`，方法：`CreateTicket(ctx, userID, req) (Ticket, error)`、`GetUserTicket(ctx, userID, ticketID) (TicketWithReplies, error)`、`ListUserTickets(ctx, userID, page, pageSize) (PaginatedTickets, error)`、`AppendUserReply(ctx, userID, ticketID, content) (Reply, error)`、`CloseUserTicket(ctx, userID, ticketID) error`、`ListAdminTickets(ctx, filter, page, pageSize) (PaginatedTickets, error)`、`GetAdminTicket(ctx, ticketID) (TicketWithReplies, error)`、`AppendAdminReply(ctx, adminID, ticketID, content) (Reply, error)`、`PatchAdmin(ctx, adminID, ticketID, patch) (Ticket, error)`、`ListCategories(ctx) ([]string, defaultPriority, error)`
- [ ] 4.2 `CreateTicket`：校验 `support_ticket_enabled = true`（否则返回特殊 ErrFeatureDisabled，handler 翻成 404）；校验 title/content/chat_context 长度上限；校验 category 在当前配置内
- [ ] 4.3 `AppendAdminReply`：在事务中写 reply + 如果当前 `status = "open"` 则一并 update 为 `in_progress`（同事务原子）；任何 `status = "closed"` 工单返回 ErrTicketClosed (handler 翻成 409)
- [ ] 4.4 `AppendUserReply`：校验工单 owner = userID；`status = "closed"` 返回 ErrTicketClosed
- [ ] 4.5 `CloseUserTicket` / admin 关闭路径：set status=closed + closed_at=now()；幂等性——已 closed 再次 close 返回 409（不是 200，避免静默掩盖）
- [ ] 4.6 `PatchAdmin`：拒绝任何把 `closed → 非 closed` 的转移（409）；priority 与 category 各自独立校验
- [ ] 4.7 单测覆盖每个方法的 happy path + 至少一个边界（disabled / not-owner / closed / invalid category）

## 5. 后端：Handler 层

- [ ] 5.1 新建 `internal/handler/support_ticket_handler.go`，挂用户端方法：`Create / List / Get / AppendReply / Close / ListCategories`
- [ ] 5.2 新建 `internal/handler/admin_support_ticket_handler.go`，挂 admin 方法：`List / Get / AppendReply / Patch`
- [ ] 5.3 错误映射：`ErrFeatureDisabled → 404 Not Found`（不暴露功能存在）；`ErrTicketClosed → 409 Conflict`；`ErrNotFound / 非 owner → 404 Not Found`（统一 404，不分 403/404 防探测）；`ErrInvalidCategory / 长度超限 → 400 Bad Request`
- [ ] 5.4 响应 DTO 类型化：`TicketDTO / TicketWithRepliesDTO / ReplyDTO`，列表 DTO 不含 `chat_context` 字段（编译期保证）
- [ ] 5.5 单测 `support_ticket_handler_test.go` + `admin_support_ticket_handler_test.go` 覆盖每一条 spec scenario

## 6. 后端：路由注册

- [ ] 6.1 新建 `internal/server/routes/support.go`，定义 `RegisterSupportRoutes(r *gin.RouterGroup, h *Handlers)`，把用户端路由挂在 `/api/v1/support` 子组（带 `RequireAuth` 中间件），admin 路由挂在 `/api/admin/support` 子组（带 `RequireAuth + RequireAdmin`）
- [ ] 6.2 在 server 总装入口（`internal/server/router.go` 或同等位置）调用 `RegisterSupportRoutes`
- [ ] 6.3 `Handlers` 聚合结构体增加 `SupportTicket *SupportTicketHandler` 与 `AdminSupportTicket *AdminSupportTicketHandler` 两个字段；wire / NewHandlers 处补构造

## 7. 后端：integration 测试

- [ ] 7.1 集成测试 `support_ticket_integration_test.go`：覆盖完整流程（create → user reply → admin reply 切 in_progress → admin patch high → user close → 再次 reply 409）
- [ ] 7.2 集成测试：feature_disabled 场景下 `POST /api/v1/support/tickets` 返回 404、admin 端路由仍可访问（admin 可以提前编辑配置）
- [ ] 7.3 跑 `go test ./internal/handler/... ./internal/service/... ./internal/repository/...` 全绿

## 8. 前端：API 客户端

- [ ] 8.1 新建 `frontend/src/api/support.ts`，导出用户端方法：`createTicket(req) / listMyTickets(page, pageSize) / getMyTicket(id) / appendReply(id, content) / closeTicket(id) / listCategories()`
- [ ] 8.2 同文件导出 admin 端方法：`adminListTickets(filter, page, pageSize) / adminGetTicket(id) / adminAppendReply(id, content) / adminPatchTicket(id, patch)`
- [ ] 8.3 定义 TS 类型：`SupportTicket / SupportTicketWithReplies / SupportTicketReply / TicketStatus / TicketPriority / AdminTicketFilter`，与后端 DTO 一一对应

## 9. 前端：用户端页面

- [ ] 9.1 新建 `frontend/src/views/support/SupportTicketsListView.vue`：列表卡片 + 分页 + "新建工单"按钮 + 空态；status / priority 用彩色 badge；表格列：title / category / status / priority / created_at
- [ ] 9.2 新建 `frontend/src/views/support/SupportTicketNewView.vue`：title / category 下拉（拉 `listCategories()` 填充）/ content (Markdown textarea) 三个字段；提交后跳到详情页；解析 URL query：当 `from=chat&session=<key>` 存在时从 `localStorage[<key>]` 读对话历史 → 拼成 Markdown 草稿填进 content + 同时塞到 hidden `chat_context` 字段；读不到时 silent skip（仅 console.warn）
- [ ] 9.3 新建 `frontend/src/views/support/SupportTicketDetailView.vue`：顶部 ticket meta（title / status / priority / category / 时间）；中部回复时间线（按 created_at 升序，user/admin 不同样式 + admin 显示为"客服"）；底部回复输入框（`status = closed` 时禁用并显示提示）；右上 "关闭工单" 按钮（仅 status ≠ closed 时显示）
- [ ] 9.4 路由注册：在 `frontend/src/router/index.ts` 加 3 条 `requiresAuth: true` 路由 `/support/tickets`、`/support/tickets/new`、`/support/tickets/:id`
- [ ] 9.5 在 `AppSidebar.vue` 用户导航与 admin 个人区都加 `{path: '/support/tickets', label: t('nav.support'), ...}` 入口；用 `appStore.cachedPublicSettings.support_ticket_enabled` 作为 featureFlag

## 10. 前端：Admin 端页面

- [ ] 10.1 新建 `frontend/src/views/admin/AdminSupportTicketsView.vue`：表格 + 顶部过滤栏（status select / category select / priority select / user_id input / q 关键词）；表格行点击打开右侧 drawer 显示 detail + 回复输入 + 状态/优先级下拉
- [ ] 10.2 路由注册：`/admin/support/tickets`，`requiresAdmin: true`
- [ ] 10.3 在 `AppSidebar.vue` admin 导航中"用量"之前插入工单菜单项 `{path: '/admin/support/tickets', label: t('nav.adminTickets'), ...}`，用 `support_ticket_enabled` 守卫
- [ ] 10.4 在 admin sidebar 工单菜单项右侧显示一个未读 badge（`open` + `in_progress` 总数），数据来自每分钟轮询 `GET /api/admin/support/tickets?status=open&page_size=1` 的 total 字段（避免新增专用 unread-count 端点）—— 如时间紧可作为可选项

## 11. 前端：Admin 设置页接入

- [ ] 11.1 在 `frontend/src/views/admin/SettingsView.vue` 的 `general` tab 内（"自定义菜单"分组之后）增加 "客服与工单" 卡片：`Toggle support_ticket_enabled` + 数组编辑器 `support_ticket_categories` + 下拉 `support_ticket_default_priority`
- [ ] 11.2 form 数据结构、保存逻辑、validation 与现有 setting 表单一致；保存成功后 toast + 刷新公共设置（让 `cachedPublicSettings` 更新）

## 12. 前端：i18n

- [ ] 12.1 `frontend/src/i18n/locales/zh.ts` 新增三组键：`support.tickets.{title,empty,newButton,statusLabel,priorityLabel,categoryLabel,...}`、`admin.tickets.{filterStatus,filterCategory,...,replyPlaceholder,closeConfirm}`、`admin.settings.support.{title,description,enabled,categories,defaultPriority,...}`
- [ ] 12.2 `frontend/src/i18n/locales/en.ts` 同步加英文
- [ ] 12.3 `nav.support` (普通用户) / `nav.adminTickets` (admin) i18n 键加在两份 locale 的 nav 命名空间下

## 13. 前端：测试

- [ ] 13.1 `SupportTicketsListView.spec.ts`：空态 / 有数据 / 分页 / 入口在 `support_ticket_enabled = false` 时不渲染
- [ ] 13.2 `SupportTicketNewView.spec.ts`：表单校验 / 提交跳详情 / `from=chat&session=k` query 存在时自动填充 / localStorage 缺失时 silent skip
- [ ] 13.3 `SupportTicketDetailView.spec.ts`：reply 时间线渲染 / closed 状态时输入框禁用 / "关闭工单" 按钮在 closed 时不显示 / admin 回复显示为"客服"
- [ ] 13.4 `AdminSupportTicketsView.spec.ts`：filter 改变触发列表请求 / drawer 打开后能改 priority 与 status / 关闭工单后 status badge 变化
- [ ] 13.5 跑 `pnpm test` / `npm test` 全绿

## 14. 联调与文档

- [ ] 14.1 本地启动后端 + 前端，端到端验证：用户提交 → admin 回复 → 用户追加 → admin 关闭 → 用户尝试再回复看到 409 toast
- [ ] 14.2 验证 `support_ticket_enabled = false` 时所有入口隐藏 + `POST /api/v1/support/tickets` 返回 404
- [ ] 14.3 验证 admin 改分类后用户新建页下拉立即更新（清前端 cache）
- [ ] 14.4 跑 `openspec validate add-support-ticket-system --strict`
- [ ] 14.5 准备 PR 描述：链接 proposal、附 schema 截图、附用户端 / admin 端关键页面截图

## 15. 归档

- [ ] 15.1 PR 合并、上线后，按 `openspec-archive-change` 流程归档 `add-support-ticket-system`，把 `support-ticket` capability spec 落入主 `openspec/specs/`
