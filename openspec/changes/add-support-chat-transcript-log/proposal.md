## Why

客服浮窗（`add-support-chat-widget` + `change-support-chat-external-llm` + `add-support-knowledge-rag`）目前是**纯透传、无状态**的：`SupportChatHandler.PostChat` 把用户消息转发到外部 LLM，再用 `streamSSEFromUpstream` 把 SSE 分片逐字节透传给浏览器，转发完即遗忘。整条链路**没有任何落库**——请求 body 里的 `session_id` 虽然已被解析（注释标注"仅用于日志/审计"），但既不写库也不用于关联。

这带来三个运营缺口：

1. **无法回溯用户问了什么、客服答了什么**——出现"AI 乱答""答非所问"投诉时无据可查。
2. **无法度量客服健康度**——上游 401（api_key 配错）、上游报错、用户中途断开这些失败，只在 `slog` 里一闪而过，管理员无法在后台看到"最近有多少次对话失败、失败在哪一步"。
3. **无法沉淀知识**——高频真实问题本可反哺 FAQ / RAG 知识库，但对话从不留痕。

工单系统（`add-support-ticket-system`）早已建立了"用户内容落库 + admin 后台列表/详情查看"的完整样板（`support_tickets` + `support_ticket_replies` 两表、`AdminSupportTicketsView`、侧边栏入口）。本 change 为客服浮窗补上对称的能力：**把对话与回包持久化，并在管理端提供只读的"客服对话记录"入口**。

## What Changes

- **新增**两张表 `support_chat_conversations`（会话头，以浏览器已在发送的 `session_id` 为业务键）+ `support_chat_messages`（逐轮消息，1:N）。落库粒度按**整段对话**：同一 `session_id` 的多轮问答归并到一个会话，管理端可查看完整对话线。
- **新增**后端落库逻辑：改造 `SupportChatHandler.PostChat`，在**每一个返回分支**（含未打到上游就早退的分支）落库当前轮的 user 消息与 assistant 回包。回包通过 **tee 累积**——`streamSSEFromUpstream` 在透传每个 `data: {...}` 分片的同时解析 `choices[].delta.content` 拼接出完整回复，流收尾时连同判定出的状态一次性写库。透传延迟与现状一致。
- **新增**每轮消息的**完整状态分类**（存于 `support_chat_messages.status`）：`success` / `upstream_auth` / `upstream_error` / `interrupted` / `rate_limited` / `config_error`。**全部状态都落库**（含限流、配置缺失这类"还没打到上游就被拦下"的），并配 `error_message` 文本列存细节。
- **新增** admin 只读端点：`GET /api/v1/admin/support/chat/conversations`（分页 + 过滤：状态 / user_id / IP / 关键词 / 时间范围）与 `GET /api/v1/admin/support/chat/conversations/:id`（会话详情，返回整段消息时间线）。挂在既有 `/api/v1/admin/support` 子组下（自带 `AdminAuthMiddleware`）。
- **新增**前端管理页 `AdminSupportChatLogsView.vue`（照抄 `AdminSupportTicketsView` 的列表 + 详情抽屉/页交互）、路由 `/admin/support/chat/conversations`、以及**侧边栏"客服对话记录"菜单项**。菜单可见性**跟随 `support_chat_enabled`**（不新增独立开关），与工单菜单同款 gating 方式。
- **新增** i18n：`zh / en` 的 `admin.supportChatLogs.*` 一组键。

## Non-goals

- **不做**管理端"介入/回复"能力——本 change 是**纯只读审计**，管理员只能查看，不能在记录里插话（要人工介入仍走工单）。
- **不改**用户浮窗前端协议——`session_id` + `messages` 早已在发送，前端无需改动。
- **不做**自动留存清理（定时 purge）——留存策略先定为"永久保留"，`design.md` 记录未来可加 TTL cron 的挂载点。
- **不做**向量化/知识反哺——把对话喂回 RAG 是后续独立 change 的事。

## Capabilities

### Modified Capabilities

- `support-chat`: 在既有"无状态 SSE 透传"契约上**新增持久化行为**——对话与回包落库、每轮状态分类的语义、tee 累积回包的协议约束、各返回分支的落库时机；并新增管理端只读查看契约（列表过滤维度、详情结构、菜单 gating）。

## Impact

- **数据库**：
  - 新增 migration `backend/migrations/177_add_support_chat_logs.sql`：建 `support_chat_conversations` + `support_chat_messages` 两表及索引（会话按 `session_id` 唯一；消息按 `conversation_id + created_at`；admin 过滤按 `last_status + last_at` 与 `user_id + last_at`）。
  - 新增 ent schema `backend/ent/schema/support_chat_conversation.go` + `support_chat_message.go`，`go generate ./ent/...` 重新生成 ent client。
- **后端**：
  - `internal/handler/support_chat_handler.go`：`streamSSEFromUpstream` 增加 tee 累积器（解析 delta.content）；`PostChat` 各返回分支接入落库；早退分支（`config_error` / `rate_limited` / `upstream_auth` / `upstream_error`）各自写库。
  - `internal/repository/support_chat_log_repo.go` 新增：`UpsertConversation` / `AppendMessage` / admin `ListConversations` / `GetConversationWithMessages`（照抄 `support_ticket_repo.go` 的 ent 用法与分页/过滤模式）。
  - `internal/service/`：新增 `SupportChatLog` domain 类型 + repository 接口 + 落库服务（拼装会话头 upsert 与消息追加，套 content 长度上限）。
  - `internal/handler/admin/`：新增 admin handler `List` / `Get`（照抄 `admin` 工单 handler）。
  - `internal/server/routes/support.go`：`registerAdminSupportRoutes` 追加 `chat/conversations` 两条 GET；`Handlers` 聚合体加 admin handler 字段，wire 重新生成。
- **前端**：
  - `frontend/src/views/admin/AdminSupportChatLogsView.vue` 新增（列表 + 详情）。
  - `frontend/src/router/index.ts` 加 `/admin/support/chat/conversations` 路由（`AdminSupportChatLogs`）。
  - 侧边栏组件加菜单项，`support_chat_enabled` gating（复用工单菜单同款 public settings 判定）。
  - `frontend/src/api/support.ts` 加 `adminListChatConversations` / `adminGetChatConversation`。
  - `frontend/src/i18n/locales/{zh,en}/` 加 `admin.supportChatLogs.*`。
- **隐私**：
  - 匿名对话（`support_chat_anonymous_llm = true`）存 `client_ip`、`user_id` 置空（`BIGINT NULL`，同 `support_ticket_replies.author_id` 语义）。
  - 单条消息 content 落库前截断到上限（`50000` 字符，与工单 `chat_context` 上限对齐）。
  - admin 列表关键词搜索不回放到响应无关字段，避免泄漏。
- **不影响**：
  - 浮窗前端行为、SSE 协议、透传延迟；
  - 外部 LLM 转发逻辑、RAG 注入、限流判定（仅在其判定结果之后追加一次落库）；
  - 工单系统数据流（仅并列新增一张菜单）。
- **测试**：
  - repo 单测：会话 upsert 幂等（同 session_id 多轮 → 单会话 + turn_count 递增）、匿名 user_id 为空、content 截断、admin 分页/过滤。
  - handler 单测：tee 累积正确抽取 delta.content；各返回分支落库出对应 status（success / upstream_auth / upstream_error / interrupted / rate_limited / config_error）；client 断开 → interrupted + 部分文本。
  - 前端 spec：列表渲染 + 状态徽章、详情时间线、菜单在 `support_chat_enabled=false` 时不显示。
- **前置依赖**：本 change 建立在 `add-support-chat-widget`（已归档，提供 `session_id` 协议与 handler）与 `change-support-chat-external-llm`（已归档，提供外部 LLM 转发）之上。
