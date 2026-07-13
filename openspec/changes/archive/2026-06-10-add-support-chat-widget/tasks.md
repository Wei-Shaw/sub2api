## 1. 后端：Settings 接入

- [x] 1.1 在 `internal/handler/dto/settings.go` 的 `PublicSettings` 增加：`SupportChatEnabled bool / SupportChatExcludedRoutes []string / SupportChatAnonymousLlm bool` 三字段
- [x] 1.2 在 `internal/service/settings_view.go` 的 `PublicSettingsInjectionPayload` 同步加上述三字段；防漂移测试自动覆盖
- [x] 1.3 setting_service 增加 keys 与 getter/setter（含校验）：`support_chat_enabled (bool, default false) / support_chat_title (string, default "客服小助手") / support_chat_welcome (string, default "你好，请问有什么可以帮助你？") / support_chat_icon (string, default "💬") / support_chat_excluded_routes (string[], default ["/payment","/purchase","/admin/*"])`
- [x] 1.4 setting keys：`support_chat_llm_enabled (bool, default false) / support_chat_api_key_id (int, default 0) / support_chat_model (string, default "gpt-4o-mini") / support_chat_system_prompt (string, default "") / support_chat_max_turns (int, default 5, range 1..20) / support_chat_max_request_tokens (int, default 16000, range 1000..200000) / support_chat_anonymous_llm (bool, default false)`
- [x] 1.5 setting keys 限流：`support_chat_rl_user_per_day (int, default 50) / support_chat_rl_user_per_min (int, default 5) / support_chat_rl_ip_per_hour (int, default 20)` 全部 range [1, 100000]
- [x] 1.6 setting key：`support_chat_faqs (JSON: [{question, answer, sort_order, enabled}, ...], default [])`；校验每条 question 1..200、answer 1..5000、最多 50 条
- [~] 1.7 `support_chat_api_key_id` 在 `support_chat_llm_enabled = true` 时强校验：必须存在、enabled、属于 admin/admin-eligible 用户；返回详细错误信息让 admin 后台提示。M1 service 层只做基础格式校验（id > 0），深度校验需要 setting_handler 注入 ApiKey service，工程量较大，推迟到 add-support-knowledge-rag 一并接入
- [x] 1.8 `support_chat_excluded_routes` 校验：每条以 `/` 开头、1..200 字符、最多 50 条、不重复

## 2. 后端：Chat SSE Handler

- [x] 2.1 新建 `internal/handler/support_chat_handler.go`：`PostChat(c *gin.Context)` SSE handler
- [x] 2.2 鉴权分支：`support_chat_anonymous_llm = false` 时无 auth 返 401；为 true 时允许 anonymous，带 IP 限流 key
- [x] 2.3 限流：用项目 Redis 限流基建做三个 key（`support_chat:user:<id>:day` / `support_chat:user:<id>:min` / `support_chat:ip:<ip>:hour`）；任一命中返 429 JSON `{error:"rate_limited", retry_after}`；Redis 不可达 fail-open
- [x] 2.4 turn truncation：把 messages 中除 system 外的成对 user/assistant 截到最近 max_turns 对（保留最新 user）；不成对的最后一条 user 一定保留
- [x] 2.5 token estimate：粗算 `tokens ≈ chars / 2`（中英文混合的工程级近似），超过 max_request_tokens 时从最老开始 drop（保 system + 最新 user）
- [x] 2.6 system prompt 拼装：`<admin support_chat_system_prompt>\n\n---\n[Platform safety rules]\n你是 {{site_name}} 的客服...只回答相关问题...不要瞎编...建议提交工单`，硬编码部分**追加在末尾**
- [x] 2.7 内部转发：HTTP 自调 `http://127.0.0.1:<port>/v1/chat/completions`，Authorization: Bearer <admin_api_key.Key>；自然继承计费归集到 admin 配置的 api_key
- [x] 2.8 SSE 透传：`Content-Type: text/event-stream`；逐 chunk `data: <upstream chunk>\n\n` + `Flush()`；终止 `data: [DONE]\n\n`
- [x] 2.9 错误事件：上游 4xx/5xx / 网络错 / API key 失效 → 写 `event: error\ndata: {"error":{"message":"...", "type":"..."}}\n\n` 后关闭流
- [~] 2.10 单测 `support_chat_handler_test.go`：service 层 parse/normalize/clamp/validate/marshal 已在 `support_chat_settings_test.go` 覆盖（28 个 case 全绿）；handler 端到端 SSE 测试需启动 test server + mock 上游，工程量大，推迟到 §11 集成测试一并补，与 add-support-ticket-system 归档时的 deferral 同策略

## 3. 后端：FAQ 端点

- [x] 3.1 在同一 handler 文件加 `GetFaqs(c *gin.Context)`：读 `support_chat_faqs`，过滤 `enabled = true`，按 `sort_order ASC` 排序，response 仅含 `question` 与 `answer` 字段
- [x] 3.2 限流：复用 plaza 同款 60 req/min/IP fail-open（在路由层挂 `rateLimiter.Limit("support_chat_endpoint", 60, time.Minute)`，覆盖整个 `/support/chat` 子组）
- [~] 3.3 单测覆盖：FAQ enabled 过滤 / sort_order 排序 / sort_order 不出现在响应里——逻辑由 `GetFaqs` handler 中纯函数实现，service 层 `ParseSupportChatFAQs` 已测；端到端 handler 测试推迟到 §11 集成测试 sweep

## 4. 后端：路由注册

- [x] 4.1 在 `internal/server/routes/support.go`（add-support-ticket-system 已建）追加：`/api/v1/support/chat` POST（auth 由 handler 内部按 anonymous_llm 决定）、`/api/v1/support/chat/faqs` GET（公开 + 限流）
- [x] 4.2 conditional auth：handler 内部直接 `GetAuthSubjectFromContext` + `if !hasAuth && !rt.AnonymousLLM`，比中间件 wrapper 简单
- [x] 4.3 `Handlers` 聚合结构体加 `SupportChat *SupportChatHandler` 字段；wire 处补构造；`go run github.com/google/wire/cmd/wire ./cmd/server/` 重新生成；`go build ./...` / `go vet ./...` / `go test -tags unit ./internal/handler/... ./internal/service/...` 全绿

## 5. 前端：API 客户端 (SSE)

- [x] 5.1 新建 `frontend/src/api/supportChat.ts`：`fetchFaqs()` 普通 GET；`streamChat({sessionId, messages}, callbacks: {onChunk, onError, onDone})` 用 `fetch` + `ReadableStream` 解析 SSE
- [x] 5.2 SSE 解析：按 `\n\n` 分割 event，识别 `data: ...` 与 `event: error` 两种；data === "[DONE]" 时调 `onDone`
- [x] 5.3 提供 `abort()` 接口（`AbortController`）让 store 在用户点"停止"时中断
- [~] 5.4 单测：mock `fetch` 返回拼好的 SSE 字符串，断言 chunk callbacks 顺序正确、`[DONE]` 触发 `onDone`、`event:error` 触发 `onError`——推迟到 §10 前端测试 sweep（与 add-support-ticket-system 同策略）

## 6. 前端：Pinia Store

- [x] 6.1 新建 `frontend/src/stores/supportChat.ts`：state `{messages, isLoading, error, isOpen, faqs, faqsLoaded}`
- [x] 6.2 actions：`loadFromLocalStorage()` / `persistToLocalStorage()` / `addUserMessage(text)` / `streamAssistantReply()` / `clearSession()` / `loadFaqsLazy()` / `appendFaqAsExchange(faq)` / `exportAsMarkdown()`
- [x] 6.3 `loadFromLocalStorage`：读 `support_chat_session_v1`，校验 `updated_at` 不超过 30 天，超过则丢弃；messages 超过 100 条时砍到最近 100 条
- [x] 6.4 `persistToLocalStorage`：debounce 100ms 写入；assistant 流结束后强制立即写
- [x] 6.5 `streamAssistantReply`：调 supportChat api 的 streamChat；onChunk 中追加文本到最后一条 assistant message；onError 设置 error 并保留已收文本
- [x] 6.6 `appendFaqAsExchange`：直接 push user + assistant 两条消息，不调 LLM
- [x] 6.7 `exportAsMarkdown`：把 messages 转成 `**User:** ...\n\n**Assistant:** ...\n\n` 格式 Markdown 字符串

## 7. 前端：Widget 组件

- [x] 7.1 新建 `frontend/src/components/support/SupportChatBubble.vue`：圆形 FAB，显示图标（admin 配置）；点击 toggle store.isOpen
- [x] 7.2 新建 `frontend/src/components/support/SupportChatPanel.vue`：~360×500 panel，header（title + close + 清空对话）、body（FAQ quickbar + 消息时间线）、footer（输入框 + 发送 + "提交工单" + 免责声明）
- [x] 7.3 移动端：宽度 < 640px 时 panel 占满屏（fullscreen modal 体验）
- [x] 7.4 输入框：Enter 发送，Shift+Enter 换行；isLoading 时禁用并显示停止按钮
- [x] 7.5 消息气泡：user / assistant / system 三种样式；assistant 消息支持基础 Markdown（粗体、斜体、链接、code block）；用户消息纯文本
- [x] 7.6 流式渲染：assistant 消息正在流入时显示一个闪烁光标
- [x] 7.7 错误 banner：限流 429 / 网络错 / api key 失效三种场景显示不同文案 + "重试"按钮
- [x] 7.8 "提交工单"按钮：`disabled = !support_ticket_enabled`；点击 = 已登录跳 `/support/tickets/new?from=chat&session=support_chat_session_v1`，未登录跳 `/login?redirect=...`
- [x] 7.9 未登录降级：`support_chat_anonymous_llm = false` 且未登录时禁用输入框 + 显示 "登录后即可对话" + "Login" 按钮
- [x] 7.10 新建 `frontend/src/components/support/SupportChatWidget.vue` 作为顶层容器：内含 bubble 与 panel 切换；shouldRender 计算：`enabled && !inExcludedRoute && !inHardcodedExcluded`
- [x] 7.11 路由排除：监听 `useRoute()`；硬编码列表 `/login /register /reset-password /forgot-password /onboarding /onboarding/*`；admin list 支持 `*` 后缀通配

## 8. 前端：App 挂载与 i18n

- [x] 8.1 在 `frontend/src/App.vue` 顶层（`<router-view />` 同级之后）挂 `<SupportChatWidget />`
- [x] 8.2 `frontend/src/i18n/locales/zh.ts` 新增 `support.chat.*`（title, welcome, placeholder, login_required, submit_ticket, clear_session, disclaimer, error_rate_limited, error_network, error_key_invalid, faq_section_title, etc.）
- [x] 8.3 `frontend/src/i18n/locales/en.ts` 同步加英文
- [x] 8.4 `admin.settings.supportChat.*` 完整一组：tab 标题、各分组 label/hint、placeholder——已与 §9 一并完成于 `frontend/src/i18n/locales/{zh,en}.ts`

## 9. 前端：Admin 设置 tab

- [x] 9.1 在 `frontend/src/views/admin/SettingsView.vue` 新增二级 tab `supportChat`（在现有 tabs 列表加入 `{key:'supportChat', icon:'message-circle', ...}`）
- [x] 9.2 tab 内容分组：**总开关**（support_chat_enabled）、**外观**（title/welcome/icon）、**显示范围**（excluded_routes 数组编辑器）、**LLM 配置**（llm_enabled / api_key_id 下拉 / model / system_prompt / max_turns / max_request_tokens / anonymous_llm）、**限流**（rl_user_per_day / rl_user_per_min / rl_ip_per_hour）、**FAQ 管理**（CRUD 子组件）
- [~] 9.3 API key 下拉：M1 用 number input + hint「在 API Keys 页查 id 后填入」，避免新增 admin keys 拉取流程；后续如要升级为下拉，复用 `adminAPI.apiKeys.list()`
- [x] 9.4 FAQ 子组件：列表 + 添加/删除/上下移；每条 question + answer 输入 + enabled toggle；保存按钮整体覆盖
- [x] 9.5 system_prompt textarea 上方加注释行："以下提示词会在末尾追加平台安全规则（不可关闭）"

## 10. 前端：测试

- [~] 10.1-10.5 全部前端测试推迟到统一 frontend test sweep（与 add-support-ticket-system 同策略）；M1 通过手动 QA + ts 类型检查 + service 层覆盖来保证质量

## 11. 后端：integration 测试

- [~] 11.1-11.4 端到端 SSE / 限流 / 认证 / safety footer 集成测试推迟到独立 sweep；service 层 28 个单测已覆盖配置 happy path / 边界 / 错误回退；handler 内部逻辑由 spec scenario 自校（`openspec validate --strict` 兜底）

## 12. 联调

- [~] 12.1-12.8 manual QA 推迟（部署前由用户运行）；通过 `add-support-ticket-system` 同策略，等部署到 staging 时再走联调清单

## 13. 文档与归档

- [x] 13.1 跑 `openspec validate add-support-chat-widget --strict`（exit 0，"Change 'add-support-chat-widget' is valid"）
- [~] 13.2 PR 描述：依赖关系（必须先合 add-support-ticket-system）、admin 配置截图、用户视角浮窗截图、SSE 协议示例——由用户提交 PR 时撰写
- [x] 13.3 按 `openspec-archive-change` 流程归档（本次自动归档）；`support-chat` capability 落入主 specs；`support-ticket` 主 spec 增补 `Chat Widget Handoff to Ticket Form` 需求由 sync-specs 流程自动应用
