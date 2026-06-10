## Why

`add-support-ticket-system` 解决了"用户找不到地方反馈"的问题，但仍要求用户"主动决定提交一个工单"——对于新手 / 简单问题（怎么充值？401 怎么解决？），工单是个重型动作。我们需要一个轻量的 always-on 入口，让用户**先尝试自助**——这就是右下角的客服浮窗。

浮窗承担两件事：(1) 收纳一个轻量的对话窗口，对登录用户做 LLM streaming 自动应答（走自己的 `/v1/chat/completions`）；(2) 在自动应答解决不了时，把当前对话历史作为草稿带去工单新建页（`add-support-ticket-system` 已经预留好接线）。

这是 **客服三件套** 的第二个 change，建立在 ticket 之上，但 LLM 应答暂时只用静态 system prompt 知识——真正的 RAG 知识库由第三个 change `add-support-knowledge-rag` 接续。本 change 故意不引入向量库，让浮窗 UI 与 LLM 透传可以独立发版可用。

## What Changes

- **新增**前端组件 `SupportChatWidget`：右下角固定浮窗（收起态 = FAB；展开态 = ~360×500 panel），全站常驻（除排除路由），带未读 badge / 输入框 / 消息时间线 / "提交工单" 按钮。
- **新增**前端 store `supportChatStore`：会话状态（messages、loading、error）、`localStorage` 持久化（key = `support_chat_session_v1`）、清空对话方法、导出对话为 Markdown 的方法（提交工单时用）。
- **新增**后端端点 `POST /api/v1/support/chat`（SSE）：对登录用户做 LLM streaming 应答；接受 `{messages, session_id}`；按 admin 配置的"客服 API key + 模型"内部转发到自身的 `/v1/chat/completions`；按本 change 配置注入硬编码 system prompt。
- **新增**后端端点 `GET /api/v1/support/chat/faqs`（公开，无需登录）：返回 admin 在客服设置里维护的"快捷 FAQ 清单"——未登录用户的 fallback 内容（仅展示 question + answer 的纯文本）。
- **新增**前端逻辑：未登录用户打开浮窗只看到 FAQ 列表 + "提交工单需登录"提示（点击跳 `/login`）；登录用户解锁 LLM 输入框。
- **新增** admin 后台二级 tab `系统设置 → 客服`：包含浮窗外观、显示范围、LLM 配置、限流、FAQ 管理、未登录策略等所有配置项。
  - 浮窗：`support_chat_enabled` (bool)、`support_chat_title`、`support_chat_welcome`、`support_chat_icon`、`support_chat_excluded_routes` (string[])。
  - LLM：`support_chat_llm_enabled` (bool)、`support_chat_api_key_id` (引用 `api_keys.id` 的下拉)、`support_chat_model` (string)、`support_chat_system_prompt` (textarea)、`support_chat_max_turns` (int, default 5)、`support_chat_max_request_tokens` (int, default 16000, hard cap)、`support_chat_anonymous_llm` (bool, default false)。
  - 限流：`support_chat_rl_user_per_day` (int, default 50)、`support_chat_rl_user_per_min` (int, default 5)、`support_chat_rl_ip_per_hour` (int, default 20)。
  - FAQ：在 `system_settings` 之外**单独**维护 `support_chat_faqs` (JSON 数组) —— 字段 `{question, answer, sort_order, enabled}`；客服 FAQ 在浮窗作为 quickbar 按钮渲染。
- **新增**前端"提交工单"按钮：用 `gotoOrLogin('/support/tickets/new?from=chat&session=support_chat_session_v1')`；新建页（`add-support-ticket-system` 中已实现）会读 localStorage 自动填充。
- **公开设置注入**：`support_chat_enabled` 进入 `PublicSettings`（控制浮窗渲染）；`support_chat_excluded_routes`、`support_chat_anonymous_llm` 同步注入（前端运行时判定）。
- **新增**全局浮窗挂载点：在 `App.vue` 顶层挂 `<SupportChatWidget />`，受路由 / 设置守卫；不挂在 `HomeView` 等具体视图内（保证全站常驻）。
- **i18n**：`zh / en` 新增 `support.chat.*`、`admin.settings.supportChat.*` 两组键。

## Capabilities

### New Capabilities

- `support-chat`: 全站客服浮窗的 UI 行为（位置、显示范围、收起展开、未读、本地持久化）、LLM 自动应答的 SSE 协议契约、未登录降级策略、FAQ 端点、限流策略、与工单系统的交接（"提交工单"按钮的语义）。

### Modified Capabilities

- `support-ticket`: 工单新建页接受的 `from=chat&session=<localStorage-key>` URL query 由"预留接线"升级为"激活语义"——本 change 落地后这个接线**真正**会被浮窗触发；新增 scenario 描述当浮窗调用此入口时新建页自动填充 `chat_context` 与 content 草稿的行为。

## Impact

- **后端**：
  - `internal/handler/support_chat_handler.go` 新增（SSE handler、FAQ handler）。
  - `internal/service/support_chat_service.go` 新增：负责拼装 system prompt、做 turns 截断、做限流计数、转发到内部 `/v1/chat/completions`（注入客服 API key 的 token），将上游 SSE 透传给客户端。
  - 路由：`internal/server/routes/support.go` 增加 `/api/v1/support/chat` (SSE, RequireAuth 或 anonymous-allowed-by-config) 与 `/api/v1/support/chat/faqs` (公开)。
  - `dto.PublicSettings` 与 `service.PublicSettingsInjectionPayload` 新增 `SupportChatEnabled bool / SupportChatExcludedRoutes []string / SupportChatAnonymousLlm bool`。
  - setting_service 增加上述所有配置项的 get/set 与校验：API key id 必须引用一条**enabled** 的 admin/admin-grade key；`max_turns ∈ [1, 20]`；`max_request_tokens ∈ [1000, 200000]`；excluded_routes 每条 1..200 字符、最多 50 条。
  - `support_chat_faqs` 单独存（同 `system_settings` 体系内的 JSON 字段），admin 端 CRUD 按数组整体保存（M1 不做单条 PATCH）。
  - 限流计数走 Redis（项目已有限流基建，复用 `RateLimiter` 中间件 + 自定义 key：`support_chat:user:<id>:day` / `support_chat:user:<id>:min` / `support_chat:ip:<ip>:hour`）。
- **数据库**：
  - 不新增表；所有配置进 `system_settings`。
- **前端**：
  - `frontend/src/components/support/SupportChatWidget.vue` 浮窗主体。
  - `frontend/src/components/support/SupportChatPanel.vue` 展开态面板（消息时间线 + 输入框 + FAQ quickbar + "提交工单"按钮）。
  - `frontend/src/components/support/SupportChatBubble.vue` 收起态 FAB。
  - `frontend/src/stores/supportChat.ts` Pinia store：messages / loading / error / `loadFromLocalStorage` / `persistToLocalStorage` / `clearSession` / `exportAsMarkdown`。
  - `frontend/src/api/supportChat.ts` SSE 客户端：用 `fetch` + `ReadableStream` 解析 SSE，回调里 push 到 store；处理上游错误事件、网络断开重连（M1 不做自动重连，仅显示错误 + 重试按钮）。
  - `frontend/src/App.vue` 顶层挂 `<SupportChatWidget />`；通过 `useRoute` 监听路由变化判定是否在 excluded 列表（默认包含 `/login`、`/register`、`/reset-password`、`/payment`、`/onboarding/*`、`/admin/*`）。
  - `frontend/src/views/admin/SettingsView.vue` 增加新二级 tab `客服`（key = `supportChat`），所有上述配置项分组排版（外观 / 显示 / LLM / 限流 / FAQ / 未登录）。
- **不影响**：
  - 工单系统数据流（仅复用其 `/support/tickets/new` 入口）；
  - 现有 `/v1/chat/completions` 端点（被本 change 内部调用，不改其行为）；
  - footer `HomeContactSection`（仍并存）；
  - 任何已有 LLM 调用（不抢用户的 API key 配额）。
- **测试**：
  - 后端 SSE handler 单测：覆盖未登录拒绝（`anonymous_llm = false`）、登录正常透传、API key 失效降级、限流命中、turns 超过 5 时截断历史、`max_request_tokens` 超出时截断最早消息。
  - 前端 widget spec：浮窗在 excluded routes 不渲染、收起/展开切换、localStorage 持久化、"提交工单"按钮跳转 URL 含 `from=chat&session=<key>`、SSE 流式渲染（用 mock stream）。
- **后续依赖**：
  - `add-support-knowledge-rag` 会**修改** `support-chat` capability：把"硬编码 system prompt"替换为"system prompt + RAG 检索结果注入"。
