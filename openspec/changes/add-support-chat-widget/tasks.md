## 1. 后端：Settings 接入

- [ ] 1.1 在 `internal/handler/dto/settings.go` 的 `PublicSettings` 增加：`SupportChatEnabled bool / SupportChatExcludedRoutes []string / SupportChatAnonymousLlm bool` 三字段
- [ ] 1.2 在 `internal/service/settings_view.go` 的 `PublicSettingsInjectionPayload` 同步加上述三字段；防漂移测试自动覆盖
- [ ] 1.3 setting_service 增加 keys 与 getter/setter（含校验）：`support_chat_enabled (bool, default false) / support_chat_title (string, default "客服小助手") / support_chat_welcome (string, default "你好，请问有什么可以帮助你？") / support_chat_icon (string, default "💬") / support_chat_excluded_routes (string[], default ["/payment","/purchase","/admin/*"])`
- [ ] 1.4 setting keys：`support_chat_llm_enabled (bool, default false) / support_chat_api_key_id (int, default 0) / support_chat_model (string, default "gpt-4o-mini") / support_chat_system_prompt (string, default "") / support_chat_max_turns (int, default 5, range 1..20) / support_chat_max_request_tokens (int, default 16000, range 1000..200000) / support_chat_anonymous_llm (bool, default false)`
- [ ] 1.5 setting keys 限流：`support_chat_rl_user_per_day (int, default 50) / support_chat_rl_user_per_min (int, default 5) / support_chat_rl_ip_per_hour (int, default 20)` 全部 range [1, 100000]
- [ ] 1.6 setting key：`support_chat_faqs (JSON: [{question, answer, sort_order, enabled}, ...], default [])`；校验每条 question 1..200、answer 1..5000、最多 50 条
- [ ] 1.7 `support_chat_api_key_id` 在 `support_chat_llm_enabled = true` 时强校验：必须存在、enabled、属于 admin/admin-eligible 用户；返回详细错误信息让 admin 后台提示
- [ ] 1.8 `support_chat_excluded_routes` 校验：每条以 `/` 开头、1..200 字符、最多 50 条、不重复

## 2. 后端：Chat SSE Handler

- [ ] 2.1 新建 `internal/handler/support_chat_handler.go`：`PostChat(c *gin.Context)` SSE handler
- [ ] 2.2 鉴权分支：`support_chat_anonymous_llm = false` 时无 auth 返 401；为 true 时允许 anonymous，带 IP 限流 key
- [ ] 2.3 限流：用项目 Redis 限流基建做三个 key（`support_chat:user:<id>:day` / `support_chat:user:<id>:min` / `support_chat:ip:<ip>:hour`）；任一命中返 429 JSON `{error:"rate_limited", retry_after}`；Redis 不可达 fail-open
- [ ] 2.4 turn truncation：把 messages 中除 system 外的成对 user/assistant 截到最近 max_turns 对（保留最新 user）；不成对的最后一条 user 一定保留
- [ ] 2.5 token estimate：粗算 `tokens ≈ chars / 2`（中英文混合的工程级近似），超过 max_request_tokens 时从最老开始 drop（保 system + 最新 user）
- [ ] 2.6 system prompt 拼装：`<admin support_chat_system_prompt>\n\n---\n[Platform safety rules]\n你是 {{site_name}} 的客服...只回答相关问题...不要瞎编...建议提交工单`，硬编码部分**追加在末尾**
- [ ] 2.7 内部转发：调用 `internal/service/support_chat_service.go` 的 `Stream(ctx, req, sink)`，service 层实现复用 `gateway_handler` 的转发逻辑（直接以 admin key id 取出真实 token 调本进程内 OpenAI handler，或开 HTTP 自调 `/v1/chat/completions` —— 选直接函数级调用以省一次往返，但若实现复杂度高则退化为 HTTP 自调）
- [ ] 2.8 SSE 透传：`Content-Type: text/event-stream`；逐 chunk `data: <upstream chunk>\n\n` + `Flush()`；终止 `data: [DONE]\n\n`
- [ ] 2.9 错误事件：上游 4xx/5xx / 网络错 / API key 失效 → 写 `event: error\ndata: {"error":{"message":"...", "type":"..."}}\n\n` 后关闭流（复用 `stream_error_event.go` 的辅助函数）
- [ ] 2.10 单测 `support_chat_handler_test.go`：覆盖 spec 各 scenario（auth / rate-limit / truncate / token cap / safety footer / Redis fail-open / upstream error）

## 3. 后端：FAQ 端点

- [ ] 3.1 在同一 handler 文件加 `GetFaqs(c *gin.Context)`：读 `support_chat_faqs`，过滤 `enabled = true`，按 `sort_order ASC` 排序，response 仅含 `question` 与 `answer` 字段
- [ ] 3.2 限流：复用 plaza 同款 60 req/min/IP fail-open
- [ ] 3.3 单测覆盖：空数组、enabled 过滤、order 排序、sort_order 字段不在响应里、Redis 限流 fail-open

## 4. 后端：路由注册

- [ ] 4.1 在 `internal/server/routes/support.go`（add-support-ticket-system 已建）追加：`/api/v1/support/chat` POST（带 conditional auth）、`/api/v1/support/chat/faqs` GET（公开 + 限流）
- [ ] 4.2 conditional auth middleware：读 `support_chat_anonymous_llm`，false 时强制 auth；为简化，可写一个小 wrapper `optionalOrRequiredAuth(settingKey)`
- [ ] 4.3 `Handlers` 聚合结构体加 `SupportChat *SupportChatHandler` 字段；wire 处补构造

## 5. 前端：API 客户端 (SSE)

- [ ] 5.1 新建 `frontend/src/api/supportChat.ts`：`fetchFaqs()` 普通 GET；`streamChat({sessionId, messages}, callbacks: {onChunk, onError, onDone})` 用 `fetch` + `ReadableStream` 解析 SSE
- [ ] 5.2 SSE 解析：按 `\n\n` 分割 event，识别 `data: ...` 与 `event: error` 两种；data === "[DONE]" 时调 `onDone`
- [ ] 5.3 提供 `abort()` 接口（`AbortController`）让 store 在用户点"停止"时中断
- [ ] 5.4 单测：mock `fetch` 返回拼好的 SSE 字符串，断言 chunk callbacks 顺序正确、`[DONE]` 触发 `onDone`、`event:error` 触发 `onError`

## 6. 前端：Pinia Store

- [ ] 6.1 新建 `frontend/src/stores/supportChat.ts`：state `{messages, isLoading, error, isOpen, faqs, faqsLoaded}`
- [ ] 6.2 actions：`loadFromLocalStorage()` / `persistToLocalStorage()` / `addUserMessage(text)` / `streamAssistantReply()` / `clearSession()` / `loadFaqsLazy()` / `appendFaqAsExchange(faq)` / `exportAsMarkdown()`
- [ ] 6.3 `loadFromLocalStorage`：读 `support_chat_session_v1`，校验 `updated_at` 不超过 30 天，超过则丢弃；messages 超过 100 条时砍到最近 100 条
- [ ] 6.4 `persistToLocalStorage`：debounce 100ms 写入；assistant 流结束后强制立即写
- [ ] 6.5 `streamAssistantReply`：调 supportChat api 的 streamChat；onChunk 中追加文本到最后一条 assistant message；onError 设置 error 并保留已收文本
- [ ] 6.6 `appendFaqAsExchange`：直接 push user + assistant 两条消息，不调 LLM
- [ ] 6.7 `exportAsMarkdown`：把 messages 转成 `**User:** ...\n\n**Assistant:** ...\n\n` 格式 Markdown 字符串

## 7. 前端：Widget 组件

- [ ] 7.1 新建 `frontend/src/components/support/SupportChatBubble.vue`：圆形 FAB，显示图标（admin 配置）；点击 toggle store.isOpen
- [ ] 7.2 新建 `frontend/src/components/support/SupportChatPanel.vue`：~360×500 panel，header（title + close + 清空对话）、body（FAQ quickbar + 消息时间线）、footer（输入框 + 发送 + "提交工单" + 免责声明）
- [ ] 7.3 移动端：宽度 < 640px 时 panel 占满屏（fullscreen modal 体验）
- [ ] 7.4 输入框：Enter 发送，Shift+Enter 换行；isLoading 时禁用并显示停止按钮
- [ ] 7.5 消息气泡：user / assistant / system 三种样式；assistant 消息支持基础 Markdown（粗体、斜体、链接、code block）；用户消息纯文本
- [ ] 7.6 流式渲染：assistant 消息正在流入时显示一个闪烁光标
- [ ] 7.7 错误 banner：限流 429 / 网络错 / api key 失效三种场景显示不同文案 + "重试"按钮
- [ ] 7.8 "提交工单"按钮：`disabled = !support_ticket_enabled`；点击 = 已登录跳 `/support/tickets/new?from=chat&session=support_chat_session_v1`，未登录跳 `/login?redirect=...`
- [ ] 7.9 未登录降级：`support_chat_anonymous_llm = false` 且未登录时禁用输入框 + 显示 "登录后即可对话" + "Login" 按钮
- [ ] 7.10 新建 `frontend/src/components/support/SupportChatWidget.vue` 作为顶层容器：内含 bubble 与 panel 切换；shouldRender 计算：`enabled && !inExcludedRoute && !inHardcodedExcluded`
- [ ] 7.11 路由排除：监听 `useRoute()`；硬编码列表 `/login /register /reset-password /forgot-password /onboarding /onboarding/*`；admin list 支持 `*` 后缀通配

## 8. 前端：App 挂载与 i18n

- [ ] 8.1 在 `frontend/src/App.vue` 顶层（`<router-view />` 同级之后）挂 `<SupportChatWidget />`
- [ ] 8.2 `frontend/src/i18n/locales/zh.ts` 新增 `support.chat.*`（title, welcome, placeholder, login_required, submit_ticket, clear_session, disclaimer, error_rate_limited, error_network, error_key_invalid, faq_section_title, etc.）
- [ ] 8.3 `frontend/src/i18n/locales/en.ts` 同步加英文
- [ ] 8.4 `admin.settings.supportChat.*` 完整一组：tab 标题、各分组 label/hint、placeholder

## 9. 前端：Admin 设置 tab

- [ ] 9.1 在 `frontend/src/views/admin/SettingsView.vue` 新增二级 tab `supportChat`（在现有 tabs 列表加入 `{key:'supportChat', icon:'message-circle', ...}`）
- [ ] 9.2 tab 内容分组：**总开关**（support_chat_enabled）、**外观**（title/welcome/icon）、**显示范围**（excluded_routes 数组编辑器）、**LLM 配置**（llm_enabled / api_key_id 下拉 / model / system_prompt / max_turns / max_request_tokens / anonymous_llm）、**限流**（rl_user_per_day / rl_user_per_min / rl_ip_per_hour）、**FAQ 管理**（CRUD 子组件）
- [ ] 9.3 API key 下拉：拉 admin 自己的 keys 列表，显示"备注 (id) [enabled/disabled]"，value = id
- [ ] 9.4 FAQ 子组件：列表 + 添加/删除/上下移；每条 question + answer 输入 + enabled toggle；保存按钮整体覆盖
- [ ] 9.5 system_prompt textarea 上方加注释行："以下提示词会在末尾追加平台安全规则（不可关闭）"

## 10. 前端：测试

- [ ] 10.1 `SupportChatWidget.spec.ts`：覆盖 (a) `support_chat_enabled=false` 不渲染 (b) `/login` 不渲染 (c) `excluded_routes=['/admin/*']` 在 `/admin/settings` 不渲染 (d) 跨路由切换状态保留
- [ ] 10.2 `SupportChatPanel.spec.ts`：mock store + i18n，覆盖 expanded / collapsed / 输入禁用（未登录 + anonymous_llm=false） / "提交工单"按钮跳转 URL 含正确 query / 流式渲染 + 错误 banner
- [ ] 10.3 `supportChat.store.spec.ts`：localStorage 持久化、30 天过期、100 条上限、addUserMessage 后 persistToLocalStorage 触发、appendFaqAsExchange 不调 LLM
- [ ] 10.4 `supportChat.api.spec.ts`：mock fetch SSE 流，断言 chunk → onChunk 顺序、`[DONE]` → onDone、`event:error` → onError
- [ ] 10.5 跑 `pnpm test` 全绿

## 11. 后端：integration 测试

- [ ] 11.1 端到端 SSE 测试：启动 test server + mock 上游，发请求 → 断言响应包含 `data: {choices...}` 与 `data: [DONE]`
- [ ] 11.2 限流命中：连发 N+1 次，第 N+1 次返 429
- [ ] 11.3 认证：anonymous_llm=false 时无 auth 返 401；=true 时通过
- [ ] 11.4 安全 footer：发请求让 mock 上游 echo 收到的 system message，断言末尾包含硬编码安全 footer 关键词

## 12. 联调

- [ ] 12.1 启动后端 + 前端，admin 配置：选客服 key + 模型 + system prompt + 启用浮窗 + 启用 LLM
- [ ] 12.2 普通用户登录后：浮窗出现 → FAQ 加载 → 多轮对话流式渲染 → 第 6 轮时观察前端是否显示截断提示（M1 可省）
- [ ] 12.3 普通用户测试限流：连发 6 条，第 6 条收到 429 与 retry_after 提示
- [ ] 12.4 退出登录：浮窗变成 FAQ 模式 + 输入禁用 + Login 按钮
- [ ] 12.5 切到 `/login` `/admin/settings` `/payment`：浮窗消失
- [ ] 12.6 工单接线：在浮窗对话 3 轮后点"提交工单" → 跳新建页 → 看 content 已被 Markdown 草稿填充
- [ ] 12.7 关闭 `support_chat_enabled` → 浮窗立即消失（刷新即可）
- [ ] 12.8 关闭 `support_chat_llm_enabled` → 浮窗仍在，输入禁用，FAQ 与提交工单仍可用

## 13. 文档与归档

- [ ] 13.1 跑 `openspec validate add-support-chat-widget --strict`
- [ ] 13.2 PR 描述：依赖关系（必须先合 add-support-ticket-system）、admin 配置截图、用户视角浮窗截图、SSE 协议示例
- [ ] 13.3 上线后按 `openspec-archive-change` 流程归档，把 `support-chat` capability 落入主 specs；`support-ticket` 主 spec 增补 `Chat Widget Handoff to Ticket Form` 需求
