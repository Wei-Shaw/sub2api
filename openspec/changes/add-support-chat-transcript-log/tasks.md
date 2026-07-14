## 1. 后端：数据模型（表 + ent）

- [x] 1.1 新建 migration `backend/migrations/177_add_support_chat_logs.sql`：建 `support_chat_conversations`（`id BIGSERIAL PK` / `session_id VARCHAR(128) NOT NULL UNIQUE` / `user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL` / `client_ip VARCHAR(64)` / `turn_count INT NOT NULL DEFAULT 0` / `last_status VARCHAR(24)` / `first_at TIMESTAMPTZ` / `last_at TIMESTAMPTZ` / `created_at` / `updated_at`）
- [x] 1.2 同 migration 建 `support_chat_messages`（`id BIGSERIAL PK` / `conversation_id BIGINT NOT NULL REFERENCES support_chat_conversations(id) ON DELETE CASCADE` / `role VARCHAR(16) NOT NULL` / `content TEXT NOT NULL` / `status VARCHAR(24)` / `error_message TEXT` / `model VARCHAR(128)` / `latency_ms INT` / `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`）
- [x] 1.3 建索引：`support_chat_conversations (last_status, last_at DESC)`、`(user_id, last_at DESC)`；`support_chat_messages (conversation_id, created_at)`。加表/列 COMMENT（含 status 取值枚举、匿名 user_id 语义、content 截断上限说明）
- [x] 1.4 migration 走 `//go:embed *.sql` 自动加载（migrations.go 无需手动注册），编号 177 顺序正确。【应用迁移验证 BLOCKED：分类器不可用，无法跑 DB】
- [x] 1.5 新建 ent schema `backend/ent/schema/support_chat_conversation.go`：`entsql.Annotation{Table:"support_chat_conversations"}`、`TimeMixin`、字段与索引对齐 migration（`user_id` Optional/Nillable、`session_id` NotEmpty、`last_status`/时间字段等）
- [x] 1.6 新建 ent schema `backend/ent/schema/support_chat_message.go`：字段对齐 migration，`status`/`error_message`/`model`/`latency_ms` 均 Optional/Nillable，`content` SchemaType text
- [ ] 1.7 `cd backend && go generate ./ent/...` 重新生成 ent client；`go build ./...` 通过 【BLOCKED：分类器不可用，无法运行 go generate / go build】

## 2. 后端：Domain + Repository

- [x] 2.1 新增 domain/service 类型：`SupportChatConversation` + `SupportChatMessage`（含 status 枚举常量：`ChatLogStatusSuccess/UpstreamAuth/UpstreamError/Interrupted/RateLimited/ConfigError`）→ `support_chat_log.go`
- [x] 2.2 定义 `SupportChatLogRepository` 接口（service 包，与 `SupportTicketRepository` 同位置）：`UpsertConversationAndAppend(ctx, turn)` / `ListConversations(ctx, filter, page)` / `GetConversationWithMessages(ctx, id)`
- [x] 2.3 新建 `internal/repository/support_chat_log_repo.go`：ent `sql/upsert` `ON CONFLICT(session_id) DO UPDATE turn_count+1, last_status/last_at=EXCLUDED`（user_id 不在 conflict 更新——同 session 身份稳定）；append user + assistant 行；**整轮包一个 ent 事务**
- [x] 2.4 `ListConversations` 分页 + 过滤（status/user_id/ip/q/from/to）：`q` 走 `HasMessagesWith(ContainsFold)` EXISTS 子查询（参数化）；列表不含消息正文；排序 `last_at DESC, id DESC`
- [x] 2.5 `GetConversationWithMessages`：取会话头 + 按 `created_at ASC` 的全部消息行
- [x] 2.6 content 落库前截断到 50000 字符 helper（`truncateChatLogContent`，rune 计数），user 与 assistant 行都套用；服务层 `SupportChatLogContentMaxLen` 常量
- [ ] 2.7 repo 单测 `support_chat_log_repo_test.go`：幂等/匿名/截断/分页过滤/级联删除 【未写：需运行 ent client + DB harness，分类器不可用无法运行，推迟到解除阻塞后一并补】

## 3. 后端：落库接线（改造 handler）

- [x] 3.1 改造 `streamSSEFromUpstream`：返回 `streamResult{Text, DoneSeen, ClientBroke, ReadErr}`；先 Write+Flush 再累积；`extractSSEDeltaContent` 抽 `choices[0].delta.content`，解析失败 silent skip；`[DONE]` 置 DoneSeen；写失败置 ClientBroke
- [x] 3.2 落库 helper `persistChatTurn(...)`：detached ctx（`context.WithoutCancel` + 5s 超时，保证 interrupted 分支仍能写）；内部调 `SupportChatLogService.PersistTurn`；**失败仅 slog.Warn**
- [x] 3.3 成功/中断路径接线：stream 返回后按 ClientBroke/ReadErr/DoneSeen 判定 `success`/`interrupted`/`upstream_error`，用累积文本落库
- [x] 3.4 早退分支接线：`config_error`（creds 缺失）、`upstream_auth`（上游 401）、`upstream_error`（上游不可达 / 非 200）各自 return 前落库
- [x] 3.5 `rate_limited` 分支接线：bind 提前到限流之前（401 未登录检查仍在 bind 之后，语义等价），三个限流分支命中各落 `rate_limited`
- [x] 3.6 不落库分支确认：`enabled=false`/`llm_enabled=false`（404）、未登录 401、messages 空/bind 失败——均不接落库
- [x] 3.7 记录 `latency_ms`（`elapsedMS()` 从 startAt 到收尾）；`model` 取 `rt.Model`
- [~] 3.8 handler 单测：tee 解析纯函数测试 `support_chat_log_tee_test.go` 已写（8 case，覆盖 malformed skip / [DONE] / 空 / 多 choice）。【端到端 status 分支测试需 mock 上游 + test server + DB，推迟到解除阻塞后补，与既有 support_chat handler 测试的 deferral 同策略】

## 4. 后端：Admin 端点 + 路由

- [x] 4.1 新建 admin handler `internal/handler/admin/support_chat_log_handler.go`：`List(c)`（分页 + status/user_id/ip/q/from/to 过滤）；`Get(c)`（:id → 会话 + 消息时间线）；含内联响应 DTO
- [x] 4.2 `internal/server/routes/support.go` 的 `registerAdminSupportRoutes` 追加 `chat.GET("/conversations")` + `chat.GET("/conversations/:id")`
- [x] 4.3 `AdminHandlers` 加 `SupportChatLog` 字段 + `ProvideAdminHandlers` 参数/装配；`SupportChatHandler` 注入 `logService`；repo/service/handler 三处 ProviderSet 补构造 + `NewSupportChatHandler` 加参。【`wire_gen.go` 重新生成 BLOCKED：分类器不可用】
- [ ] 4.4 `go build ./... && go vet ./... && go test ./internal/repository/... ./internal/handler/...` 全绿 【BLOCKED：分类器不可用】

## 5. 前端：API + 视图 + 路由 + 菜单

- [x] 5.1 `frontend/src/api/support.ts` 加 `adminListChatConversations` + `adminGetChatConversation` + 类型（`ChatLogStatus`/`ChatConversationListItem`/`ChatConversationMessage`/`ChatConversationDetail`/`AdminChatLogFilter`）
- [x] 5.2 新建 `frontend/src/views/admin/AdminSupportChatLogsView.vue`：列表（分页 + status/user_id/ip/q 过滤 + 状态徽章）+ 详情 Dialog（消息时间线，user/assistant 气泡，assistant 行显示 status/error/model/latency）
- [x] 5.3 状态徽章：6 种 status 内联 `statusClass`/`statusLabel`（success 绿 / 上游+鉴权错误 红 / 限流+配置+中断 黄）
- [x] 5.4 `frontend/src/router/index.ts` 加路由 `/admin/support/chat/conversations`（name `AdminSupportChatLogs`，admin 守卫，lazy import）
- [x] 5.5 侧边栏加"客服对话记录"菜单项：新增 `FeatureFlags.supportChat`（key `support_chat_enabled`, opt-in）+ `flagSupportChat`，菜单项跟随该 flag
- [x] 5.6 i18n：`zh/en` 加 `admin.supportChatLogs.*`（title/subtitle/col/filter/status/detail/空态）+ `nav.adminSupportChatLogs`

## 6. 前端测试

- [ ] 6.1 `AdminSupportChatLogsView.spec.ts`：列表渲染 + 状态徽章映射；详情时间线渲染；过滤器触发请求参数正确 【未写：推迟到解除阻塞后补，需 vitest 运行验证】
- [ ] 6.2 侧边栏 spec：`support_chat_enabled=true` 显示菜单、`=false` 不显示 【未写：同上】
- [ ] 6.3 `pnpm -C frontend test` / `pnpm -C frontend build`（或项目既定命令）全绿 【BLOCKED：分类器不可用】

## 7. 验证与收尾

- [ ] 7.1 端到端手测：开浮窗发问 → 后台"客服对话记录"看到会话；连问多轮 → 同一会话 turn_count 递增；配错 api_key → 记录出 `upstream_auth`；触发限流 → 记录出 `rate_limited` 【BLOCKED：需运行服务】
- [ ] 7.2 匿名模式（`support_chat_anonymous_llm=true`）手测：记录 user_id 为空、有 IP 【BLOCKED：需运行服务】
- [ ] 7.3 确认透传延迟无回归（tee 是"先写后累积"）【BLOCKED：需运行服务】
- [ ] 7.4 `openspec validate add-support-chat-transcript-log --strict` 通过 【BLOCKED：分类器不可用】
