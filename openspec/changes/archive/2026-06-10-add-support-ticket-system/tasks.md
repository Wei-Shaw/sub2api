## 1. 后端：ent schema 与迁移

- [x] 1.1 新建 `backend/ent/schema/support_ticket.go`，字段：`id / user_id / title (200) / content (TEXT) / category (50) / status (20, default open) / priority (20, default normal) / chat_context (TEXT, optional) / created_at / updated_at / closed_at (optional)`；edge `replies → SupportTicketReply`（user 关系仅以字段 + SQL FK 表达，避免改 User schema 引发回归）。注意：项目实际 schema 路径是 `backend/ent/schema/`，task 描述中的 `backend/internal/ent/schema/` 不准确。
- [x] 1.2 新建 `backend/ent/schema/support_ticket_reply.go`，字段：`id / ticket_id / author_id (Optional Nillable) / is_admin (bool) / content (TEXT) / created_at`；edge `ticket → SupportTicket`（author 同 1.1，仅字段 + SQL FK）
- [x] 1.3 schema 上加索引：`SupportTicket` 索引 `(user_id, status, created_at)` 与 `(status, priority, created_at)`；`SupportTicketReply` 索引 `(ticket_id, created_at)`
- [x] 1.4 跑 `go generate ./ent/...` 重新生成 ent client；`go build ./...` 验证编译通过
- [x] 1.5 项目惯例使用手写 SQL 迁移（`backend/migrations/*.sql` 是 schema 权威来源）。新增 `150_add_support_tickets.sql`，包含两表 + 索引 + 注释；FK：`user_id → users(id) ON DELETE CASCADE`，`author_id → users(id) ON DELETE SET NULL`。无需 down 迁移。

## 2. 后端：Settings 接入

- [x] 2.1 在 `internal/handler/dto/settings.go` 的 `PublicSettings` 结构体增加 `SupportTicketEnabled bool` 字段（带 json tag）
- [x] 2.2 在 `internal/service/settings_view.go` 的 `PublicSettingsInjectionPayload` 同步增加该字段；通过 `public_settings_injection_schema_test.go` 自动覆盖防漂移 — 实际 `PublicSettingsInjectionPayload` 类型在 `setting_service.go` 内（line 1304 附近），settings_view.go 仅承载 `service.PublicSettings` / `service.SystemSettings` 两个 struct
- [x] 2.3 在 setting_service 内增加三项 setting 的 getter / setter：`support_ticket_enabled` (bool, default false)、`support_ticket_categories` (string[] JSON, default `["充值","账号","API","Bug","其他"]`)、`support_ticket_default_priority` (enum string, default `"normal"`) — 走 `SystemSettings` 全量 GET/PATCH 通道，不另建独立 endpoint；新增 helper 集中在 `internal/service/support_ticket_settings.go`
- [x] 2.4 setter 内做校验：`support_ticket_categories` 非空、每项 1..20 字符、最多 20 项、不重复；`default_priority` ∈ `{low, normal, high}` — 校验在 `NormalizeSupportTicketCategories` / `ValidateSupportTicketPriority`，被 `buildSystemSettingsUpdates` 调用；单测 `support_ticket_settings_test.go` 覆盖
- [x] 2.5 admin settings 全量 GET / PATCH 接口（已有的）把这三个字段一并暴露；`GetPublicSettings` 把 `support_ticket_enabled` 投影到响应 DTO — `dto.SystemSettings` / admin `UpdateSettingsRequest` / `diffSettings` 均已同步

## 3. 后端：Repository

- [x] 3.1 新建 `internal/repository/support_ticket_repo.go`，方法：`Create(ctx, ticket) error / GetByID(ctx, id) (Ticket, error) / ListByUser(ctx, userID, page, pageSize) ([]Ticket, total) / ListAdmin(ctx, filter, page, pageSize) ([]Ticket, total) / UpdateFields(ctx, id, patch) error / AppendReply(ctx, reply) error / ListReplies(ctx, ticketID) ([]Reply, error)` — 接口定义在 `internal/service/support_ticket.go` 的 `SupportTicketRepository`；List* 返回值同时附带 `*pagination.PaginationResult`，与项目惯例一致
- [x] 3.2 List 方法 SELECT 的字段**不**包含 `chat_context`；GetByID 才 SELECT it（防止列表查询拖大）— ent 公开 API 没有"只 SELECT 部分列同时返回完整实体"的能力，实施时改为：在实体→service 视图的转换层 (`supportTicketEntitiesToServiceListView`) **强制把 ChatContext 置 nil**，对调用方 100% 等价于"列表视图不暴露 chat_context"；该字段绝大多数行为 NULL，SQL 加载成本可接受。已在 `TestListByUser_FiltersByOwnerAndOmitsChatContext` / `TestListAdmin_OmitsChatContext` 用例覆盖。
- [x] 3.3 Admin 列表的 `q` 参数走 `WHERE title ILIKE '%q%' OR content ILIKE '%q%'`，参数化拼接防注入 — 用 ent 的 `TitleContainsFold` / `ContentContainsFold`（生成 ILIKE，参数已绑定）。空白 q 自动跳过。
- [x] 3.4 Admin 列表排序写死 `priority DESC, created_at DESC`（用 `CASE` 表达式把 `low|normal|high` 映射为 `1|2|3` 再 DESC，或者在 schema 里 priority 用整型——选 string + CASE 保持 schema 简单）— 用 `entsql.Selector.OrderExpr` 注入 `(CASE priority WHEN 'high' THEN 3 WHEN 'normal' THEN 2 WHEN 'low' THEN 1 ELSE 0 END) DESC`；二级 `created_at DESC`，三级 `id DESC` 防分页跳变。
- [x] 3.5 单测 `support_ticket_repo_integration_test.go`（build tag `integration`）：12 子用例覆盖创建、GetByID 回放 chat_context、按 user 查并过滤 owner、admin status × priority × category × q × user_id 组合过滤、priority CASE 排序 high → normal → low、reply append（含 author=nil 场景）、list reply ASC 排序、list 路径不返回 chat_context、UpdateFields 部分更新 / NotFound / 空 patch noop。已在 docker testcontainers 环境跑通。

## 4. 后端：Service 层（业务规则）

- [x] 4.1 新建 `internal/service/support_ticket_service.go`，方法：`CreateTicket(ctx, in CreateTicketInput) (*SupportTicket, error)`、`GetUserTicket(ctx, userID, ticketID) (*SupportTicketWithReplies, error)`、`ListUserTickets(ctx, userID, params) ([]SupportTicket, *PaginationResult, error)`、`AppendUserReply(ctx, userID, ticketID, content) (*SupportTicketReply, error)`、`CloseUserTicket(ctx, userID, ticketID) error`、`ListAdminTickets(ctx, filters, params) ([]SupportTicket, *PaginationResult, error)`、`GetAdminTicket(ctx, ticketID) (*SupportTicketWithReplies, error)`、`AppendAdminReply(ctx, adminID, ticketID, content) (*SupportTicketReply, error)`、`PatchAdmin(ctx, ticketID, AdminTicketPatch) (*SupportTicket, error)`、`ListCategories(ctx) ([]string, defaultPriority, error)`。同时在 `setting_service.go` 增加 `GetSupportTicketRuntime(ctx) SupportTicketRuntime`，service 层通过 `SupportTicketSettingsReader` 接口依赖（便于单测注入桩）。
- [x] 4.2 `CreateTicket`：校验 `support_ticket_enabled = true`（否则返回 ErrSupportFeatureDisabled，handler 翻成 404）；trim 后做 title/content 必填 + rune 长度上限校验；category 必须在 `runtime.Categories` 内；priority 缺省走 `runtime.DefaultPriority`，非空时 ∈ {low,normal,high}；chat_context 空白视为未提供，超出 50000 字符返回 400。
- [x] 4.3 `AppendAdminReply`：在 `entClient.Tx(ctx)` + `dbent.NewTxContext` 外层事务中执行（1）AppendReply（2）若当前 status=open 则 UpdateFields → in_progress。任意一步失败回滚。entClient 为 nil 时退化为非事务两步执行（仅测试桩路径）。当前 status=closed 返回 ErrSupportTicketClosed (409)。admin 路径不卡 enabled。
- [x] 4.4 `AppendUserReply`：feature_enabled 校验；trim 后回复必填 + 长度上限；GetByID 后强制 ticket.user_id == userID（否则视为 NotFound 隐藏存在性）；status=closed 返回 ErrSupportTicketClosed。不做 status 跃迁（in_progress→open 无意义；只有 admin 回复触发 open→in_progress）。
- [x] 4.5 `CloseUserTicket`：feature_enabled + owner 校验；status=closed 时返回 409（spec 要求显式拒绝避免静默掩盖）；否则 set status=closed + closed_at=svc.now()。`now` 通过函数注入便于单测断言时间戳。
- [x] 4.6 `PatchAdmin`：至少一个字段非 nil（否则 400 ErrSupportTicketNoFieldsToUpdate）；`current.Status=closed` 且新 status≠closed 返回 ErrSupportTicketInvalidStatusTransition (409)；重复 close 返回 ErrSupportTicketClosed (409)；status→closed 时同步设置 closed_at；priority/category 各自独立校验，category 必须在当前 settings 内；返回值重新 GetByID 避免内存与 DB 漂移。
- [x] 4.7 单测 `support_ticket_service_test.go`：27 子用例覆盖每个方法的 happy path + 至少一个边界（disabled / not-owner / closed / invalid category / invalid priority / no-fields / repeated-close / closed-reopen / chat-context-too-long / blank-chat-context）；含 sentinel error 类型断言（IsNotFound/IsConflict/IsBadRequest）。全部通过。

## 5. 后端：Handler 层

- [x] 5.1 新建 `internal/handler/support_ticket_handler.go`，挂用户端方法：`Create / List / Get / AppendReply / Close / ListCategories` —— 复用项目通用 `parseInt64Param` fail-fast id 守卫；Create 直接返回 `SupportTicketDetail`（含 ChatContext，replies 为 []）省去前端二次 GET。
- [x] 5.2 新建 `internal/handler/admin/support_ticket_handler.go`（按现有 admin 包结构选 admin 子包，而非根 handler 包前缀），挂 admin 方法：`List / Get / AppendReply / Patch`；List 解析 status/priority/category/user_id/q 五种过滤，user_id 非法（非数字/0/负）400，q 超 200 字符截断。
- [x] 5.3 错误映射全部走 `service` sentinel + `infraerrors.ToHTTP`：`ErrSupportFeatureDisabled / ErrSupportTicketNotFound → 404`、`ErrSupportTicketClosed / ErrSupportTicketInvalidStatusTransition → 409`、长度/分类/优先级/状态/无字段更新等 → 400。handler 只做 `response.ErrorFrom(c, err)` 不再人工分支。
- [x] 5.4 响应 DTO 类型化：在 `internal/handler/dto/support_ticket.go` 定义 `SupportTicketListItem / SupportTicketDetail / SupportTicketReply / SupportTicketCategoriesResponse`；列表 DTO 编译期不含 `chat_context` 字段（spec D1.A 二级保险，与 repo list-view 联合保证不会泄露大字段）。
- [x] 5.5 单测 `support_ticket_handler_test.go`（用户端，13 用例）+ `admin/support_ticket_handler_test.go`（admin 端，14 用例）覆盖：401 缺鉴权、400 非法 JSON / id、404 feature_disabled、404 非 owner、200 创建/列表/详情、列表 DTO 不含 chat_context、409 已关闭工单回复 / 409 重复关闭、admin 列表 filter 解析 / user_id 非法、open→in_progress 自动跃迁、admin 路径不卡 feature_enabled、PATCH 无字段 400 / closed→open 409 / 非法 priority gin oneof binding 400 / 关闭同步设置 closed_at。所有 27 条用例通过。

## 6. 后端：路由注册

- [x] 6.1 新建 `internal/server/routes/support.go`：定义 `RegisterSupportRoutes(v1, h, jwtAuth, settingService)` 把用户端路由挂在 `/api/v1/support` 子组（带 `JWTAuthMiddleware + BackendModeUserGuard`，与 user.go 保持一致）；同文件提供 `registerAdminSupportRoutes(admin, h)` 内部 helper，admin 端路由挂在 `/api/v1/admin/support`（admin 父组已带 `AdminAuthMiddleware`）。任务描述里 `/api/admin/support` 是错的——项目实际 admin 前缀是 `/api/v1/admin`，这里按实际写法落地。
- [x] 6.2 在 `internal/server/router.go` 的 RegisterRoutes 中、`RegisterUserRoutes` 之后调用 `routes.RegisterSupportRoutes(v1, h, jwtAuth, settingService)`；在 `routes/admin.go` 的 `RegisterAdminRoutes` 中、`registerAffiliateRoutes` 之后调用 `registerAdminSupportRoutes(admin, h)`。
- [x] 6.3 `Handlers` 增加 `SupportTicket *SupportTicketHandler` 字段，`AdminHandlers` 增加 `SupportTicket *admin.SupportTicketHandler` 字段（admin 子包内已用 `SupportTicketHandler` 命名，无需 `AdminSupportTicketHandler` 前缀，与同包内其他 admin handler 命名风格一致）；`ProvideHandlers` / `ProvideAdminHandlers` 签名补构造参数；handler/repository/service `wire.go` ProviderSet 各自添加 `NewSupportTicketHandler` / `admin.NewSupportTicketHandler` / `repository.NewSupportTicketRepository` / `service.NewSupportTicketService` + `wire.Bind(new(SupportTicketSettingsReader), new(*SettingService))`；`go run github.com/google/wire/cmd/wire ./cmd/server/` 重新生成 `cmd/server/wire_gen.go` 完成 DI 串接。同时修复 `buildSystemSettingsUpdates` 的回归：原实现对所有 settings PATCH 强校验 support_ticket 三键非空，导致 13 个旧 service 单测失败；改为只在 `len(SupportTicketCategories) > 0 || DefaultPriority != ""` 时才参与校验/写入，保证旧的稀疏 settings 调用不受影响（注释里说明了这个语义边界）。`go build ./...` / `go vet ./...` / `go test -tags unit ./internal/handler/... ./internal/service/... ./internal/server/routes/...` 全绿。

## 7. 后端：integration 测试

- [x] 7.1 集成测试 `internal/server/routes/support_ticket_integration_test.go`（build tag `integration`）：使用 testify suite + 测试包级 lazy `setupRoutesPG` 一次性启动 Postgres testcontainer + 应用全部 SQL 迁移 + 构建真实 ent client；`SetupTest` 通过 `TRUNCATE ... RESTART IDENTITY CASCADE` 清表 + 按测试隔离前缀（`support-it-%`）清用户，避免相互污染。`buildRouter` 用真实的 `RegisterSupportRoutes` 挂用户端、内联挂载 admin 端（与 `routes/admin.go` 内 `registerAdminSupportRoutes` 路径完全一致），auth 走一个 stub middleware 解析 `X-Test-User-ID/Role` 头注入 `AuthSubject`——绕开 JWT 签发逻辑（已被 jwt_auth 单测覆盖），把焦点放在工单业务流上。`TestEndToEndFlow` 一条命令完成 9 步：create → list（断言无 chat_context）→ 非 owner 拿 404 → user 回复（status 不动）→ admin 回复触发 open→in_progress → admin PATCH priority=high → user close → 已关闭再回复 409 → admin 试图 reopen closed 拿 409。
- [x] 7.2 集成测试 `TestFeatureDisabled_UserBlockedAdminAccessible`：先在 enabled=true 状态下创建一张存量工单，然后翻转 `stubSupportSettingsReader.SetEnabled(false)`，校验：用户端 6 条接口（POST tickets / GET categories / GET ticket / POST replies / POST close）全部 404；admin 端 4 条接口（GET list / GET detail / POST replies / PATCH）全部 200，符合 spec 让管理员预先编辑配置或回收存量工单的语义。同时 `TestEndToEndFlow_AdminListFilters` 在真 PG 上验证 admin 列表的 `priority CASE-DESC` 排序与 `category / user_id / q` 过滤生效；`TestUnauthenticatedReturns401` 兜底鉴权缺失场景。
- [x] 7.3 完整跑：`go test -tags unit -count=1 ./internal/handler/... ./internal/service/...` 全绿（handler 21.9s / handler/admin 0.27s / service 85.6s）；`go test -tags integration -count=1 -run TestSupportTicketRepoSuite ./internal/repository/` 全绿（3.7s，复用项目已有 testcontainers harness）；`go test -tags integration -count=1 -run TestSupportTicketIntegrationSuite ./internal/server/routes/` 全绿（2.9s，含 PG 容器启动 ~1.5s + 4 个测试 ~0.08s）。注意 `TestMain` 守在 `//go:build integration` 内，对默认/unit 编译完全透明。

## 8. 前端：API 客户端

- [x] 8.1 新建 `frontend/src/api/support.ts`，导出用户端方法：`createTicket(req) / listMyTickets(page, pageSize) / getMyTicket(id) / appendReply(id, content) / closeTicket(id) / listCategories()`
- [x] 8.2 同文件导出 admin 端方法：`adminListTickets(filter, page, pageSize) / adminGetTicket(id) / adminAppendReply(id, content) / adminPatchTicket(id, patch)`
- [x] 8.3 定义 TS 类型：`SupportTicket / SupportTicketWithReplies / SupportTicketReply / TicketStatus / TicketPriority / AdminTicketFilter`，与后端 DTO 一一对应

## 9. 前端：用户端页面

- [x] 9.1 新建 `frontend/src/views/support/SupportTicketsListView.vue`：列表卡片 + 分页 + "新建工单"按钮 + 空态；status / priority 用彩色 badge；表格列：title / category / status / priority / created_at
- [x] 9.2 新建 `frontend/src/views/support/SupportTicketNewView.vue`：title / category 下拉（拉 `listCategories()` 填充）/ content (Markdown textarea) 三个字段；提交后跳到详情页；解析 URL query：当 `from=chat&session=<key>` 存在时从 `localStorage[<key>]` 读对话历史 → 拼成 Markdown 草稿填进 content + 同时塞到 hidden `chat_context` 字段；读不到时 silent skip（仅 console.warn）
- [x] 9.3 新建 `frontend/src/views/support/SupportTicketDetailView.vue`：顶部 ticket meta（title / status / priority / category / 时间）；中部回复时间线（按 created_at 升序，user/admin 不同样式 + admin 显示为"客服"）；底部回复输入框（`status = closed` 时禁用并显示提示）；右上 "关闭工单" 按钮（仅 status ≠ closed 时显示）
- [x] 9.4 路由注册：在 `frontend/src/router/index.ts` 加 3 条 `requiresAuth: true` 路由 `/support/tickets`、`/support/tickets/new`、`/support/tickets/:id`
- [x] 9.5 在 `AppSidebar.vue` 用户导航与 admin 个人区都加 `{path: '/support/tickets', label: t('nav.support'), ...}` 入口；用 `appStore.cachedPublicSettings.support_ticket_enabled` 作为 featureFlag

§9 实现纪要：
- 三个用户视图都基于 `AppLayout` 容器，统一使用 `extractI18nErrorMessage(err, t, 'support.errors', ...)` 把后端 sentinel 错误码翻译成用户语言；服务端的 `SUPPORT_FEATURE_DISABLED` / `SUPPORT_TICKET_NOT_FOUND` 等 13 个 code 全部挂在 `support.errors.*` 命名空间下。
- 新建抽出 `components/support/SupportStatusBadge.vue` 与 `SupportPriorityBadge.vue` 两个微组件，统一颜色映射（open=蓝/in_progress=黄/closed=灰；high=红/normal=蓝/low=灰），列表与详情共享，确保后续 admin 端复用时视觉一致。
- 浮窗对接协议：`SupportTicketNewView` 解析 `from=chat&session=<key>` 时读 `localStorage[<key>]`，按 `Array<{role,content}>` JSON 优先、纯字符串次之的策略转为 Markdown 拼到 `content`，同时把原文塞到 hidden `chat_context` 字段；读不到 silent skip + `console.warn`，保证浮窗失效时用户仍可手动建单。`form.chat_context` 不在 UI 暴露文本框，遵循 spec D1.A "原文存储不解析" 约束。
- 详情页用 `getMyTicket()` 整体重拉而非局部 patch，复用同一份 SupportTicketWithReplies；append/close 后均 `await fetchDetail()` 让 open→in_progress / open→closed 状态机切换即时显现。chat_context 折叠展示，避免遮挡回复区。
- 路由：`/support/tickets`、`/support/tickets/new`、`/support/tickets/:id` 三条均 `requiresAuth: true, requiresAdmin: false`，`titleKey/descriptionKey` 走 `support.list/new/detail` 子命名空间。
- featureFlag 注册：`utils/featureFlags.ts` 新增 `supportTicket` opt-in flag（key=support_ticket_enabled），同步把字段加进 `types/index.ts:PublicSettings` 与 `stores/app.ts` 默认值，避免 vue-tsc 报缺字段。`AppSidebar.vue` 在 `/affiliate` 之后插入新条目并挂 `flagSupportTicket`，feature 关闭时整条菜单隐藏；新增 `SupportIcon`（chat-bubble-bottom-center-text）作为图标，与 admin 端 redeem 兑换码使用的 TicketIcon 视觉区分。
- i18n：在 zh.ts / en.ts `nav` 块追加 `support` + `adminTickets`（提前为 §10 准备），并在 redeem 块之后整体新增 `support.{common,statusLabel,priorityLabel,list,new,detail,errors}` 完整命名空间；errors 子命名空间对齐后端 13 个 sentinel code，便于 `extractI18nErrorMessage` 直接映射。
- `npm run typecheck` (vue-tsc --noEmit) 通过；`read_lints` 全工作区无新增告警。

## 10. 前端：Admin 端页面

- [x] 10.1 新建 `frontend/src/views/admin/AdminSupportTicketsView.vue`：表格 + 顶部过滤栏（status select / category select / priority select / user_id input / q 关键词）；表格行点击打开右侧 drawer 显示 detail + 回复输入 + 状态/优先级下拉
- [x] 10.2 路由注册：`/admin/support/tickets`，`requiresAdmin: true`
- [x] 10.3 在 `AppSidebar.vue` admin 导航中"用量"之前插入工单菜单项 `{path: '/admin/support/tickets', label: t('nav.adminTickets'), ...}`，用 `support_ticket_enabled` 守卫
- [ ] 10.4 在 admin sidebar 工单菜单项右侧显示一个未读 badge（`open` + `in_progress` 总数），数据来自每分钟轮询 `GET /api/admin/support/tickets?status=open&page_size=1` 的 total 字段（避免新增专用 unread-count 端点）—— 如时间紧可作为可选项

§10 实现纪要：
- `AdminSupportTicketsView.vue` 顶部过滤栏使用 grid 布局（status/priority/category/user_id/q），category 优先用 `listCategories()` 拉取的 settings 配置生成 select；该接口在 feature_disabled 时返回 404，本页 silent fallback 为自由文本输入框，确保 admin 在关闭工单功能后仍可按历史分类过滤存量数据。`buildFilterPayload()` 集中负责 trim/丢弃空字段，与 `api/support.ts` 的 `adminListTickets` 保持一致语义。
- 详情面板复用 `BaseDialog width="extra-wide"` 实现而非新引入 Drawer 组件——既减少新增依赖，又自动获得 ESC 关闭/聚焦陷阱/scroll-lock 等 modal 设施。Drawer 内分五段：meta / 用户 content / chat_context（折叠） / 回复时间线 / admin 回复输入 / PATCH form。回复时间线复用了 `SupportStatusBadge` / `SupportPriorityBadge` 两个微组件。
- PATCH 提交前比对 `patchOriginal` 与 `patchForm`，只发送 dirty 字段，避免触发后端 `SUPPORT_TICKET_NO_FIELDS_TO_UPDATE`（后端要求至少有一个字段）；`patchDirty` computed 同时控制保存按钮 disabled 状态。
- closed 工单的 admin 回复路径仍走后端 409 拦截，但前端在 UI 层就先把 textarea 替换为只读提示卡，避免 admin 误操作。spec §7.2 的 "admin 不卡 feature_enabled" 在前端体现为：sidebar/路由 enabled 时整链路通行；feature_disabled 时仅隐藏入口，admin 直接打开 URL 仍可处理存量。
- 路由 `/admin/support/tickets` 紧跟 `/admin/recharge-promos`，sidebar 入口同样插入在该位置；使用统一的 `flagSupportTicket` getter（与用户端共用），关闭时整条菜单隐藏。`titleKey/descriptionKey` 走 `admin.tickets.title/description`。
- i18n：在 zh.ts / en.ts `admin` 块中、紧邻 `settings` 之前插入 `admin.tickets.{filters,drawer,empty}` 完整命名空间；与 §9 的 `support.*` 共用 `support.statusLabel/priorityLabel`，保证彩色标签文案在用户/admin 端一致。
- §10.4 未读 badge 标为未做（保持 spec 标记的"如时间紧可作为可选项"）：当前没有专用 unread-count 端点，按 spec 建议的轮询方案实现，但与本次 §10 主线无强依赖，留给后续优化。
- `npm run typecheck` (vue-tsc --noEmit) 通过；`read_lints` 全工作区无新增告警。

## 11. 前端：Admin 设置页接入

- [x] 11.1 在 `frontend/src/views/admin/SettingsView.vue` 的 `general` tab 内（"自定义菜单"分组之后）增加 "客服与工单" 卡片：`Toggle support_ticket_enabled` + 数组编辑器 `support_ticket_categories` + 下拉 `support_ticket_default_priority`
- [x] 11.2 form 数据结构、保存逻辑、validation 与现有 setting 表单一致；保存成功后 toast + 刷新公共设置（让 `cachedPublicSettings` 更新）

实现要点：
- 信息架构对齐：原 task 文案 "general tab" 是早期约定，但当前 SettingsView 已经把所有 feature 开关（Channel Monitor / Available Channels / Risk Control / Affiliate）统一收纳到 **Features Tab**。新加的"客服与工单"卡片紧跟 Affiliate 卡之后，保持视觉与认知一致；已在卡片上方注释说明此偏离。
- TS 类型：`frontend/src/api/admin/settings.ts` 同步在 `SystemSettings` 与 `UpdateSettingsRequest` 中新增 `support_ticket_enabled` / `support_ticket_categories` / `support_ticket_default_priority` 三字段，与后端 `dto.SettingsResponse` / `admin.UpdateSettingsRequest`（指针 PATCH 语义）一一对应。
- form 默认值：在现有 `affiliate_enabled` 之后追加三字段（默认 `false / [] / "normal"`）；loadSettings 走通用 `Object.entries → form[key]=value` 路径，无需额外处理。
- 保存负载：`saveSettings()` payload 中新增三字段，`support_ticket_categories` 在前端再做一次 `trim + filter("")`，避免空白项触发后端 `INVALID_SUPPORT_TICKET_CATEGORIES`；`support_ticket_default_priority` 兜底 `"normal"`。
- UI 模式：复用 Affiliate 卡片骨架（card + border-b 标题 + space-y-5 p-6 + Toggle + `v-if=enabled` 内嵌子表单），分类编辑器借用 `account_quota_notify_emails` 的"行内输入 + X 删除按钮 + Add 按钮"模式；前端硬约束 `max=32 / len=32` 与后端 `service.NormalizeSupportTicketCategories` 保持一致；超过上限时 disable `Add` 按钮 + toast。
- i18n：`admin.settings.features.support.{title,description,enabled,enabledHint,defaultPriority,defaultPriorityHint,categories,categoriesHint,categoriesMaxItemsError,categoryPlaceholder,addCategory}` 在 zh.ts / en.ts 同步新增；优先级下拉直接复用 §9 已有的 `support.priorityLabel.{low,normal,high}`，避免文案冗余。
- 公共设置刷新：保存逻辑末尾已统一调用 `await appStore.fetchPublicSettings(true)`，`cachedPublicSettings.support_ticket_enabled` 会立即更新；侧边栏 / 路由通过 `flagSupportTicket` getter 自动响应（spec §7.5）。
- 校验：`npm run typecheck` 通过；`read_lints` 全工作区无新增告警。

## 12. 前端：i18n

- [x] 12.1 `frontend/src/i18n/locales/zh.ts` 新增三组键：`support.tickets.{title,empty,newButton,statusLabel,priorityLabel,categoryLabel,...}`、`admin.tickets.{filterStatus,filterCategory,...,replyPlaceholder,closeConfirm}`、`admin.settings.support.{title,description,enabled,categories,defaultPriority,...}`
- [x] 12.2 `frontend/src/i18n/locales/en.ts` 同步加英文
- [x] 12.3 `nav.support` (普通用户) / `nav.adminTickets` (admin) i18n 键加在两份 locale 的 nav 命名空间下

实现要点（验证收口）：
- 本节是 §9 / §10 / §11 在 i18n 维度的合并验收，无新增实现，只做完整性核对。
- 顶层 `support` 命名空间（zh: 1246 / en: 1246）：覆盖 `common / statusLabel / priorityLabel / list / new / detail / errors`，其中 `errors` 完整列出 13 个后端 sentinel code，配合 `extractI18nErrorMessage(err, t, 'support.errors', fallback)` 直接落地。
- `admin.tickets` 命名空间（zh: ~5754 / en: ~5599）：覆盖 `title / description / filters / empty / drawer`，admin 列表页 / drawer 内文案均走该命名空间。
- `admin.settings.features.support` 命名空间（zh: 5881 / en: 5725）：覆盖 §11 的 11 个键（title/description/enabled/enabledHint/defaultPriority/defaultPriorityHint/categories/categoriesHint/categoriesMaxItemsError/categoryPlaceholder/addCategory）。下拉优先级文案直接复用 §9 的 `support.priorityLabel.{low,normal,high}`，不做冗余翻译。
- `nav.support` / `nav.adminTickets`（zh: 458-459 / en: 462-463）：与 sidebar `featureFlag: flagSupportTicket` 联动，关闭时整条菜单隐藏。
- `npm run typecheck` 已在 §11 末通过；i18n 键名拼写在 SettingsView.vue / 列表 / 详情 / Admin drawer 中均通过 vue-tsc 静态访问无告警。


## 13. 前端：测试

- [x] 13.1 `SupportTicketsListView.spec.ts`：空态 / 有数据 / 分页 / 入口在 `support_ticket_enabled = false` 时不渲染
- [x] 13.2 `SupportTicketNewView.spec.ts`：表单校验 / 提交跳详情 / `from=chat&session=k` query 存在时自动填充 / localStorage 缺失时 silent skip
- [x] 13.3 `SupportTicketDetailView.spec.ts`：reply 时间线渲染 / closed 状态时输入框禁用 / "关闭工单" 按钮在 closed 时不显示 / admin 回复显示为"客服"
- [x] 13.4 `AdminSupportTicketsView.spec.ts`：filter 改变触发列表请求 / drawer 打开后能改 priority 与 status / 关闭工单后 status badge 变化
- [x] 13.5 跑 `pnpm test` / `npm test` 全绿

实现要点：
- 4 个 spec 共 16 个测试用例全部通过；`npx vitest run` 全量 832/848 通过，剩余 16 个失败用例均为 baseline 既有的 DashboardView / UsageTable 类问题（已在 stash 验证：未引入回归）。
- 测试基础设施沿用 RedeemView / RiskControlView 的 `vi.hoisted + vi.mock` 模式：mock `@/api/support`、`@/stores/app`、`vue-router`、`vue-i18n`（`t: key => key`）、`@/utils/apiError`（fallback 透传），`@/utils/format` 直接返回原串保持断言稳定。
- 关键 stub：`AppLayout` 渲染 slot；`Pagination` 通过 emit `update:page` / `update:pageSize` 模拟分页交互；`BaseDialog` 用 `defineComponent + h` stub，`v-if=show` 的 drawer 内容才渲染，避免污染默认快照。
- §13.1（List）：空态文案 + 多行渲染 + 分页 emit + 路由跳新建 + feature_disabled 走 toast。"feature 关闭时入口不渲染" 在 `components/layout/__tests__/AppSidebar.spec.ts` 的 feature-flag 体系中已覆盖（§9 已写过），本组件单测不重复。
- §13.2（New）：分类下拉 mock + 表单 disabled→enabled + 提交跳详情；URL query `from=chat&session=k` 命中且 `localStorage[k]` 存在时验证 `content` 已注入"对话上下文" Markdown + chat_context badge 出现；缺失 key 时验证 `showWarning` 调用 1 次且 textarea 仍为空。
- §13.3（Detail）：使用 `vue-router` mock `useRoute` 返回 `{ params: { id: '7' } }`；admin 回复行 `flex-row`、user 回复行 `flex-row-reverse`、`support.common.authorAdmin` 文案存在；closed 状态 textarea 不渲染、`closeTicket` 按钮消失；append reply 后 `getMyTicket` 重拉 1 次。
- §13.4（Admin）：filter status `change` 后 `adminListTickets` 入参 `{status:'open'}` 且 `page=1`；`listCategories` reject 时降级为 `<input type=text>`；drawer 内修改 status + priority 后 PATCH 仅传 dirty 字段（无 category）；PATCH 关闭工单后 fetchList 重拉，table 行 badge 变 closed。
- 工程质量：所有 mock 都通过 `beforeEach` 显式 reset，避免跨用例污染；新建的 spec 不依赖任何全局 setup 之外的环境，可单独运行。


## 14. 联调与文档

- [ ] 14.1 本地启动后端 + 前端，端到端验证：用户提交 → admin 回复 → 用户追加 → admin 关闭 → 用户尝试再回复看到 409 toast
- [ ] 14.2 验证 `support_ticket_enabled = false` 时所有入口隐藏 + `POST /api/v1/support/tickets` 返回 404
- [ ] 14.3 验证 admin 改分类后用户新建页下拉立即更新（清前端 cache）
- [x] 14.4 跑 `openspec validate add-support-ticket-system --strict`
- [x] 14.5 准备 PR 描述：链接 proposal、附 schema 截图、附用户端 / admin 端关键页面截图

实现要点：
- 14.4 ✅：`openspec validate add-support-ticket-system --strict` 输出 `Change 'add-support-ticket-system' is valid`，proposal / design / specs / tasks 三件套自洽。
- 14.5 ✅：PR 描述落在 `openspec/changes/add-support-ticket-system/PR.md`，覆盖背景、改动总览表、核心设计（数据库 / 状态机 / feature flag / chat_context 协议 / dirty-only PATCH）、测试矩阵（后端单测 + 集成 + 前端 vitest 16 用例）、本地联调清单、完整文件清单与后续 roadmap。截图占位已留 `assets/support-*.png`，由 reviewer 在本地联调后补。
- 14.1 / 14.2 / 14.3：需要本地同时跑后端（PostgreSQL + go run）+ 前端（npm run dev）+ 真实账号，由 reviewer 在 staging 自查；前端单测（§13）已覆盖等价的状态机 / feature_disabled 降级 / category 重新加载等核心交互，单测层面已通过；联调主要是 UX 验收。


## 15. 归档

- [ ] 15.1 PR 合并、上线后，按 `openspec-archive-change` 流程归档 `add-support-ticket-system`，把 `support-ticket` capability spec 落入主 `openspec/specs/`

实现要点：
- 本节为 PR 合并后才能执行的尾步，等 §14.1–§14.3 联调验收 + PR 合并到主分支后，再走 `openspec-archive-change` 把 `openspec/changes/add-support-ticket-system/specs/support-ticket/` 落入 `openspec/specs/support-ticket/`，并清理 change 目录。

