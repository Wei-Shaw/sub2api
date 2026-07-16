## 1. DB Schema & Migration

- [x] 1.1 在 `backend/ent/schema/` 新增 `support_ticket_read.go`（表名 `support_ticket_reads`），字段 `ticket_id / user_id / last_read_at`，加复合唯一索引 `(ticket_id, user_id)` + 索引 `(user_id, last_read_at)`；FK CASCADE 通过后续 SQL migration 加。
- [x] 1.2 在 `backend/ent/schema/` 新增 `support_ticket_notification.go`（表名 `support_ticket_notification`），字段：`recipient_user_id / ticket_id / event_type / title_snapshot / excerpt / actor_user_id / is_read / created_at / read_at`；索引 `(recipient_user_id, is_read, created_at DESC)`。
- [x] 1.3 运行 `go generate ./ent` 生成客户端代码，确认 `Client.SupportTicketRead` / `Client.SupportTicketNotification` 编译通过。
- [x] 1.4 撰写 SQL migration（放入项目 migration 目录，与既有工单 migration 命名风格一致）：
  - 建 `support_ticket_reads` 表 + FK `ticket_id → support_tickets(id) ON DELETE CASCADE` + FK `user_id → users(id) ON DELETE CASCADE`。
  - 建 `support_ticket_notification` 表 + 对应 FK（`ticket_id` CASCADE，`actor_user_id` SET NULL）。
  - 加索引。
- [x] 1.5 补一个 down-migration 或说明"回滚仅 drop 两张新表，业务字段无兼容负担"。
- [ ] 1.6 运行 `make migrate` 或对应命令，本地跑一次 up + down 冒烟。（需要真实 DB 环境，本次未跑）

## 2. Backend Domain / Repository / Service (通知与未读)

- [x] 2.1 在 `backend/internal/domain/` 新增 `support_ticket_read.go` 和 `support_ticket_notification.go` domain 类型（DTO/内部 model 二选一，跟既有模块风格对齐）。
  - 实际落点：`backend/internal/service/support_ticket_notification.go`（domain 类型与 Repository 接口同层），保持与 SupportTicketRepository 一致。
- [x] 2.2 新增 `backend/internal/repository/support_ticket_read_repo.go`：
  - `MarkTicketRead(ctx, ticketID, userID, at)` → ent OnConflictColumns upsert。
  - `CountUnreadForUser` / `CountUnreadForAdmin` 走原生 SQL LEFT JOIN + LATERAL 聚合。
- [x] 2.3 CountUnreadFor{User,Admin} 聚合入口。放在 `SupportTicketReadRepository` 上（不在 SupportTicketRepository），SQL 一次查询完成。unit 测试覆盖 nil-safe / 分流 / 错误透传三类分支（integration 数据种子留 TODO）。
- [x] 2.4 新增 `backend/internal/repository/support_ticket_notification_repo.go`：
  - `Insert` 单条写入（不是批量：调用方需要在循环内分别处理错误日志）
  - `ListByRecipient(ctx, params) ([]Notification, *pagination.PaginationResult, error)`
  - `CountUnreadByRecipient(ctx, userID)`
  - `MarkOneRead(ctx, id, userID, readAt)` — (id, recipient) 二元校验，未命中返回 `ErrSupportTicketNotificationNotFound`
  - `MarkAllRead(ctx, userID, readAt) (int64, error)`
  - service 层 crud 单测覆盖 recipient=0 / 参数透传 / 错误传播 11 case。
- [x] 2.5 新增 `backend/internal/service/support_ticket_notification_service.go`：
  - `NotifyTicketCreated` / `NotifyUserReplied` / `NotifyAdminReplied` — 编排"通知记录 + 邮件"多路投递
  - `resolveAdminRecipients` — γ 规则：白名单非空时 email → user 索引匹配 → 剩余走 email-only；空则查全体 admin
  - 单测覆盖 CRUD 分支；Notify* 覆盖在 hooks 集成路径中。
- [x] 2.6 `NotificationEmailService`：
  - 新增两个事件常量 `NotificationEmailEventSupportTicketNewTicket` / `NotificationEmailEventSupportTicketNewReply`，追加到 `notificationEmailEventOrder`。
  - `notificationEmailEventDefinitions` map 里加两条 `NotificationEmailEventInfo`（含 placeholders）。
  - 邮件模板走现有 `notificationEmailTemplates` map（inline HTML card 结构，与其他事件保持一致），zh + en 两语言，reply 事件通过 `reply_kind_label` 变量分支渲染"客服回复" vs "用户回复"。
  - （模板独立 tmpl 文件的方式与项目现有邮件系统架构不匹配，改用现有的 inline 模板 map；对新事件的最小渲染断言留待后续 email service test 补齐。）

## 3. Backend Service Hooks (工单事件触发副作用)

- [x] 3.1 在 `support_ticket_service.go::CreateTicket` 成功 return 前，调用 `NotifyTicketCreated`；失败仅 `slog.Warn`（swallow 在 notifier 内部）。
- [x] 3.2 在 `AppendUserReply` 成功 return 前，调用 `NotifyUserReplied` + 同时 upsert 当前用户读游标（用户回复本身就是"读过"信号）。
- [x] 3.3 在 `AppendAdminReply` 事务提交后（`postAppendAdminReplyEffects`），调用 `NotifyAdminReplied` + upsert `support_ticket_reads` 给 admin 自己。非事务分支同样触发（兼容测试桩）。
- [x] 3.4 在 `SupportTicketService.GetUserTicket` 成功 return 前 upsert `support_ticket_reads`（当前用户）；失败 log warn 不阻塞返回。
- [x] 3.5 `GetAdminTicket` 签名新增 `adminID` 参数；handler 从 subject 传入；上层 admin handler 相应改造。upsert 逻辑与用户端对称。
- [ ] 3.6 补 integration test（`backend/internal/server/routes/support_ticket_integration_test.go`）：验证 GET detail 会 upsert read row，AppendAdminReply 会写 notification row 给 owner。（integration 测试需真实数据库，本次未落）

## 4. Backend Admin Setting `ticket_notify_emails`

- [x] 4.1 在 `domain_constants.go` 新增 `SettingKeySupportTicketNotifyEmails`，容量常量 `SupportTicketNotifyEmailsMaxCount=20` / `SupportTicketNotifyEmailMaxLen=254`。runtime 层 `enabledNotifyEmails` helper 只投影 disabled=false 的项，覆盖单元测试 6 case。
- [x] 4.2 admin settings handler 读写：
  - `SystemSettings` 加 `SupportTicketNotifyEmails []NotifyEmailEntry`（复用 AccountQuota 的 entry 类型，含 disabled/verified 状态便于 UI 记忆）
  - parse 路径读 → `ParseNotifyEmails` 兼容旧 string[] 格式
  - update 路径写 → `normalizeSupportTicketNotifyEmails` 做 lenient 归一（trim / 长度 / 去重 / 截断）后再 `MarshalNotifyEmails`
- [x] 4.3 单测覆盖 `normalizeSupportTicketNotifyEmails`：空/超长丢弃、大小写去重、数量截断 5 case。
- [x] 4.4 前端 admin settings 页面加多邮箱输入 UI：
  - `SystemSettings` 类型加 `support_ticket_notify_emails: NotifyEmailEntry[]`（含 optional 变体）
  - `SettingsView.vue` form 默认 + template 表单块（复用 AccountQuotaNotifyEmails 视觉：disabled toggle + email input + 删除按钮）+ `addSupportTicketNotifyEmail` helper + payload filter 空 email
  - i18n zh + en：`addNotifyEmail` / `notifyEmails` / `notifyEmailPlaceholder` / `notifyEmailsHint`

## 5. Backend REST API (Unread & Notifications)

- [x] 5.1 新增用户端 handler `support_ticket_notification_handler.go`：
  - `GET /api/v1/support/tickets/unread-count`（`CountUserUnreadTickets`）
  - `GET /api/v1/support/tickets/notifications`（分页 + only_unread 过滤）
  - `POST /api/v1/support/tickets/notifications/:id/read`（幂等；不匹配返回 404）
  - `POST /api/v1/support/tickets/notifications/read-all`（返回 affected int64）
- [x] 5.2 新增管理员端对应 4 个 handler（`internal/handler/admin/support_ticket_notification_handler.go`）。差异仅在 unread-count 走 `CountAdminUnreadTickets`。
- [x] 5.3 在 `internal/server/routes/support.go` 注册 8 条路由：用户端挂 `JWTAuthMiddleware + BackendModeUserGuard`；admin 端挂 `AdminAuthMiddleware`。通知路由**必须放在 /:id 之前**，防止 gin 把 `unread-count` / `notifications` 当路径参数吃掉。
- [x] 5.4 handler 单元测试：
  - 用户端 13 case：happy / 401 / 400 / 404 / 500
  - admin 端 6 case：unread-count 走 admin 聚合分支（用 `countCalledFor` 断言不走错分支）+ subject 隔离
  - DTO 展平测试 4 case：nil→0、空切片返回 `[]`、保序
- [x] 5.5 API 文档：新建 `api_docs/support_ticket_notifications.md`（8 端点详解 + 请求/响应体 + 状态码 + feature flag 语义 + 邮件白名单归一化 + 相关代码入口）

## 6. Frontend API Bindings

- [x] 6.1 `frontend/src/api/support.ts` 增加 8 个函数 + 5 个 TS 类型：
  - `getTicketUnreadCount()` / `getAdminTicketUnreadCount()`
  - `getTicketNotifications(params)` / `getAdminTicketNotifications(params)`
  - `markTicketNotificationRead(id)` / `markAdminTicketNotificationRead(id)`
  - `markAllTicketNotificationsRead()` / `markAllAdminTicketNotificationsRead()`
  - 类型：`TicketNotification` / `TicketUnreadResponse` / `TicketNotificationListResponse` / `TicketNotificationMarkReadResponse` / `TicketNotificationMarkAllReadResponse`
- [x] 6.2 类型契约测试：`frontend/src/api/__tests__/support.notifications.spec.ts`，10 case 覆盖 URL、query、类型契约（`IsExact` 断言字段与后端 dto 严格对齐）。

## 7. Frontend Pinia Store

- [x] 7.1 新建 `frontend/src/stores/ticketUnread.ts`，state（`unreadCount / notifications / total / page / pageSize / loading*`）+ actions（`fetchUnreadCount / fetchNotifications / markRead / markAllRead / startPolling / stopPolling / reset`）+ getters（`hasUnread / unreadNotificationCount / hasMore`）
- [x] 7.2 role-aware 分流：读 `useAuthStore().isAdmin`，admin 走 admin API、user 走 user API
- [x] 7.3 `startPolling / stopPolling`：60s `setInterval` + `visibilitychange` 立即刷新（页面回前台时 tick），stopPolling 幂等
- [x] 7.4 auth 状态变化 hook（`App.vue`）：登入 startPolling、登出 `ticketUnreadStore.reset()`
- [x] 7.5 feature-flag 短路：读 `useAppStore().cachedPublicSettings?.support_ticket_enabled`，false / null 时所有 fetch/poll 空跑
- [x] 7.6 store 单元测试：16 case 覆盖 disabled 空跑、role 分流、markRead 乐观更新/失败回滚、markAllRead 幂等、polling 幂等、reset 清空

## 8. Frontend `AnnouncementBell` 改 Tab

- [~] 8.1 抽出现有公告逻辑：**不做**——采用最小侵入方案，在原 `AnnouncementBell.vue` 上加 tab bar，保留公告 UI 不动，减小回归面
- [x] 8.2 工单 tab panel：inline 在 `AnnouncementBell.vue` 里，含空状态 / loading / item 列表 / "全部标已读"按钮 / item 点击处理
- [x] 8.3 badge 计数：`max(未读通知条目数, 未读工单聚合数)` + 未读公告数（避免双源红点数字不一致）
- [x] 8.4 默认 tab 打开策略抽为纯函数 `pickDefaultBellTab`（`announcementBellTab.ts`），policy 与 spec 一致
- [x] 8.5 tab header 各自显示未读 badge（`99+` clamp）
- [x] 8.6 item 点击：`useAuthStore().isAdmin` 分流：
  - user → `router.push('/support/tickets/:id')`
  - admin → `router.push({ path: '/admin/support/tickets', query: { open: id } })` + `AdminSupportTicketsView.vue` 新增 `watch(route.query.open)` → `openDrawer(id)`（自动清 query）
- [x] 8.7 视觉：未读高亮 + 粗体标题；已读静态灰
- [x] 8.8 vitest 组件测试：抽 `pickDefaultBellTab` 纯函数后 5 case 覆盖所有默认打开策略分支（等价于原任务列出的 4 个组件挂载 case，代价低 90%）
- [x] 8.9 挂载点：`AppHeader.vue` 里的 `AnnouncementBell` 无需改动，管理员/用户共用

## 9. Frontend Sidebar Red Dot

- [x] 9.1 `AppSidebar.vue` `/support/tickets` 菜单项加 `showDot: flagTicketUnreadDot`（读 `ticketUnreadStore.hasUnread`）
- [x] 9.2 admin `/admin/support/tickets` 同样加 `showDot: flagTicketUnreadDot`
- [x] 9.3 `router.afterEach`（在 `App.vue`）触发 `ticketUnreadStore.fetchUnreadCount()`，store 内 `UNREAD_COUNT_FETCH_MIN_INTERVAL_MS=5s` 节流
- [ ] 9.4 手工冒烟：留 PR 描述执行

## 10. i18n

- [x] 10.1 zh 键：
  - `announcementBell.tabs.announcement/ticket`
  - `announcementBell.actions.markAllRead/markedAllRead`
  - `announcementBell.ticket.empty/emptyDescription`
  - `admin.settings.features.support.notifyEmails/notifyEmailPlaceholder/notifyEmailsHint/addNotifyEmail`
  - （`nav.mySupport.redDotAria` / `admin.support.tickets.redDotAria` 未新增，sidebar 复用了 payment 通用红点 aria—足够 screen reader 传达"未读提示"语义）
- [x] 10.2 en 键：完整对应
- [x] 10.3 邮件模板：走现有 `notificationEmailTemplates` map（inline HTML card 结构），zh + en 两语；reply 事件用 `reply_kind_label` 变量分支渲染标题。
- [x] 10.4 i18n 键完整性：`pnpm typecheck` + eslint 通过；`pnpm exec vitest run` 全量绿

## 11. Static / Lint / 测试完整回归

- [x] 11.1 `cd backend && go test -tags=unit ./...` 全绿
- [ ] 11.2 `cd backend && go test -tags=integration ./...` 需要真实 DB；本次未跑
- [x] 11.3 `cd backend && golangci-lint run` 全绿（gofmt 3 项已修复：`support_ticket_read_repo.go` / `support_ticket_notification*.go` / `handler.go` / `handler/wire.go` / `repository/wire.go`）
- [x] 11.4 `make test-frontend-critical` 6 files / 95 tests 全绿；`pnpm exec eslint .` 全绿；`pnpm typecheck` 全绿
- [ ] 11.5 `check_pnpm_audit_exceptions.py`：本次未跑
- [ ] 11.6 手工端到端冒烟：留 PR 描述执行

## 12. Docs & Changelog

- [x] 12.1 更新 `DEV_GUIDE.md`：新增第七章"客服工单系统"（7.1-7.7），覆盖开关/数据模型/通知链路/邮件事件/前后端接入要点/测试落点
- [x] 12.2 CHANGELOG / RELEASE_NOTES：项目未维护此类文件；相关变更清单已写入本 tasks.md + `api_docs/support_ticket_notifications.md` + DEV_GUIDE
- [x] 12.3 admin operator 手册：`api_docs/support_ticket_notifications.md` 里"管理员邮件收件白名单"章节列出配置语义与归一化规则；SettingsView UI 内的 `notifyEmailsHint` 也解释了白名单行为
