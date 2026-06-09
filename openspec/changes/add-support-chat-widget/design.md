## Context

项目核心是一个 LLM 网关：自身就提供 OpenAI 兼容的 `/v1/chat/completions` 端点，已经有完备的 SSE 流式上行/下行机制（参考 `internal/handler/gateway_handler.go`、`stream_error_event.go`、`user_msg_queue_helper.go`）。客服浮窗的"脑子"只是这套机制的**第二个内部消费者**——把 admin 配置的"客服 API key + 模型"作为请求凭证，注入 system prompt，然后把 token 流透传给浮窗 UI。

约束：
- 全站常驻（`App.vue` 挂载，受路由 / 设置守卫排除特定路径）。
- 未登录用户**不能**调 LLM（除非 admin 显式开 `support_chat_anonymous_llm`），但能看 FAQ 列表 + 跳转登录。
- 单会话最多 5 轮（admin 可配 1..20）；超出截断最早的消息。
- 单次请求 hard cap 16k tokens（admin 可配）；估算+截断在后端做。
- 流式响应（SSE），前端逐 chunk 渲染。
- 浮窗对话 localStorage 持久化（key = `support_chat_session_v1`）。
- 工单系统已存在（依赖 `add-support-ticket-system`）；本 change 通过 `/support/tickets/new?from=chat&session=<key>` 把对话历史交接给工单新建页。
- 暂不做 RAG（留给 `add-support-knowledge-rag`）；本 change 的 system prompt 来自 admin 后台 textarea 的硬编码内容。

## Goals / Non-Goals

**Goals：**

- 全站右下角始终有一个可见的客服入口，登录用户能流畅地多轮对话。
- 未登录用户也能看到入口，打开后能浏览 FAQ + 提示登录（不让流量溜走）。
- Admin 在系统设置里能完整配置浮窗外观、LLM 行为、限流、FAQ 列表、未登录策略，无需改代码。
- 自动应答的"脑子"完全复用项目自身的 `/v1/chat/completions`，吃自己狗粮。
- 浮窗与工单系统在用户侧形成连贯链路：解决不了 → 一键带对话历史去提工单。

**Non-Goals：**

- 不做向量检索 / 真 RAG（属于 `add-support-knowledge-rag`）。
- 不做主动招呼（"X 秒后弹气泡"）—— 用户明确不要。
- 不做多设备同步 / 服务端会话存储 ——`localStorage` 即可，刷新页面续上，跨设备不续。
- 不做语音 / 文件上传。
- 不做 markdown 渲染中的复杂功能（只支持基础粗体、链接、代码块；GFM 不必要）。
- 不做 admin 端"客服会话监控面板" —— 隐私敏感且超本期范围。
- 不做自动重连：网络中断时显示错误 + 重试按钮。

## Decisions

### D1. 浮窗挂载点：`App.vue` 顶层，不挂在每个 view

**选择**：`App.vue` 在 `<router-view />` 之后挂 `<SupportChatWidget />`。

**理由**：
- 全站常驻 = 不能由具体 view 决定显隐；
- 路由切换时浮窗的内部状态（展开/收起、对话）保持不重建；
- 受 `useRoute` 监听 + `support_chat_excluded_routes` 配置 + `support_chat_enabled` 设置三重守卫决定是否渲染。

### D2. 显示守卫：三层叠加

```
渲染前提：
  support_chat_enabled === true                          (admin 总开关)
  AND
  current path ∉ support_chat_excluded_routes            (路由排除)
  AND
  current path ∉ DEFAULT_HARDCODED_EXCLUDES              (硬编码排除)
```

**硬编码默认排除**（admin 不可关闭，避免误操作）：
- `/login`、`/register`、`/reset-password`、`/forgot-password`
- `/onboarding`、`/onboarding/*`（driver.js 已经霸屏）
- 支付回调路径 `/auth/wechat/payment/callback` 等

**Admin 可配置排除**（`support_chat_excluded_routes`，默认值）：
- `/payment` 与 `/purchase`（支付页面，浮窗 + 误触体验差）
- `/admin/*`（admin 自己看自己客服心智割裂）

**匹配规则**：每条配置支持精确等于 + `*` 后缀通配（`/admin/*` = `startsWith('/admin/')`）。

### D3. SSE 协议：透传 OpenAI 风格

```
POST /api/v1/support/chat
Content-Type: application/json

{
  "session_id": "client-generated-uuid",
  "messages": [
    {"role":"user", "content":"..."},
    {"role":"assistant", "content":"..."},
    ... (max 5 turns = 最多 10 条 message)
  ]
}

→ 200 OK
  Content-Type: text/event-stream

data: {"choices":[{"delta":{"content":"你"}}]}

data: {"choices":[{"delta":{"content":"好"}}]}

...

data: [DONE]
```

**关键决策**：
- **不**自创 event 名称（`event: token` 之类），直接复用 OpenAI 兼容的 `data: {choices...}` 格式。前端解析逻辑可以直接抄已有 OpenAI gateway 客户端代码。
- 错误用项目已有的 `event: error\ndata: {error:{message,...}}\n\n` 终止事件（参考 `stream_error_event.go`）。
- `session_id` 仅做请求级追踪 / 限流计数（不写库），后端 `RequestID` 中间件会 log 它便于排查。

### D4. 后端转发链路

```
SupportChatHandler (SSE)
   │
   │ 1. RequireAuth (除非 anonymous_llm = true)
   │ 2. 限流检查 (Redis: user:day, user:min, ip:hour)
   │ 3. 截断 messages 到 admin 配置的 max_turns
   │ 4. 估算 tokens（粗算：每个 char 0.5 tokens 上限），
   │    超过 max_request_tokens 时从最早开始 drop（保 system + 最新 user）
   │ 5. 拼装 system message:
   │      project context (硬编码字符串模板，参数 = site_name, doc_url)
   │      + admin 配置的 support_chat_system_prompt
   │      + (knowledge-rag change 落地后会插入检索片段，本 change 留 hook)
   │ 6. 组装 OpenAI ChatCompletion 请求 (stream=true)
   │ 7. 用 admin 配置的 support_chat_api_key_id 对应的 hashed key 作为 Authorization
   │ 8. 调用本进程内的 InternalChatCompletion 函数（绕开 HTTP 自调，省一次往返）
   │    这个函数复用 gateway_handler 的转发逻辑但不写计费日志？—— 见 D5
   │ 9. 把上游 SSE chunk 透传给客户端 (io.Copy 或 chunked write + flush)
   │
   ▼
   Internal: 走 gateway 同款链路 → 上游 LLM provider
```

### D5. 计费 / 用量归属

**问题**：客服 API key 是某个 admin 在 admin 后台勾选的（如 admin 自己的 key）。当用户在浮窗用了 LLM，token 消耗算到谁头上？

**决定**：算到那个被选中的 key 头上（即客服 key 的 owner 账户）。

**理由**：
- 浮窗用 LLM 是"平台运营成本"，不是用户消耗；本来就是平台 sponsor 给用户的；
- 平台 owner 通过 admin 后台选了哪个 key 就用哪个 key 的余额——逻辑清晰；
- 不另起一套计费豁免机制 = 简单。

**实现细节**：内部转发请求时携带的 token 就是该 key 的真实 token，因此 `gateway_handler` 已有的计费/usage tracking 自然把消耗记到该 key。**不需要**给 `gateway_handler` 加旁路。

**风险**：
- 滥用 → 由限流 + max_turns + max_tokens 三层兜底；
- 客服 key 余额不足 → 返回 SSE error 事件，前端显示"客服暂时不可用，可提交工单"。

### D6. 客服 key 的选择 UI

```
admin 后台 → 系统设置 → 客服 → LLM 区块:

  使用的 API Key:  [下拉选择 ▼]
                   options:
                     · 列出 admin 自己拥有的 keys（key id + 备注）
                     · 显示 key 是否 enabled
                     · 显示 key 当前余额（如有）
                   value 存的是 api_keys.id (整数)，不是 raw token
```

**校验**：保存时后端检查 key 必须存在、enabled、属于 admin / admin-eligible 用户（避免选了一个普通用户的 key）。校验失败返回 400 给 admin 后台。

### D7. 限流策略

```
默认值（admin 可配）：
  user_per_day:  50      （每日上限）
  user_per_min:  5       （爆发限制）
  ip_per_hour:   20      （未登录降级到 FAQ 时无效；anonymous_llm=true 时启用）

实现：
  Redis INCR + EXPIRE 三个 key
  超过任一 → 返回 429 Too Many Requests (JSON, 不是 SSE)
  错误体含 retry_after 字段
  前端 toast 提示并禁用输入 retry_after 秒
```

**fail-open**：Redis 不可达时**不**阻断（与项目其它端点一致），仅降级失去限流能力。

### D8. 单会话 5 轮的具体含义

```
"轮" = 一次 user → assistant 完整往返
5 轮 = 最多 10 条消息 + 1 条 system

超过时：
  保留 最近 5 轮（10 条），
  最老的丢弃，
  前端在被截断时显示一行灰色提示 "已开启新一轮对话"

为什么不开新会话：
  用户体感上是连续的，但 LLM 上下文已经只看最近 5 轮 —— 这是工程妥协，UI 侧让用户感知较弱
```

### D9. 未登录策略：`support_chat_anonymous_llm`

```
support_chat_anonymous_llm = false (默认):
  浮窗对未登录用户:
    - 显示标题 + 欢迎语
    - 显示 FAQ quickbar
    - 输入框被禁用，placeholder = "登录后即可对话"
    - 输入框下方有 "登录" 按钮 → /login?redirect=<current>
    - "提交工单"按钮 → /login?redirect=/support/tickets/new

support_chat_anonymous_llm = true:
  浮窗对未登录用户:
    - 同上但输入框启用
    - LLM 限流走 ip_per_hour
    - 提示文案："匿名对话不会保留" (清浏览器即没了 — 实际上 localStorage 仍在，
      但出于隐私模糊处理)
```

### D10. FAQ：单独存，不进 PublicSettings

```
support_chat_faqs (JSON array):
  [
    {
      "question": "怎么充值？",
      "answer":   "进入 /payment 页面，选择金额后...",
      "sort_order": 0,
      "enabled":  true
    },
    ...
  ]

  读：GET /api/v1/support/chat/faqs (公开, 无 auth)
      响应只返回 enabled = true 的 + 按 sort_order 排序
  写：admin 后台保存时整体覆盖（M1 不做单条 PATCH）
```

**理由**：
- 不进 PublicSettings：FAQ 内容可能很长（每条 100..1000 字 × 多条 = 几十 KB），把它放 PublicSettings 会让每个用户首屏都拉一次大 payload；
- 单独的 GET 端点：浮窗第一次展开时再 lazy 拉。

### D11. 前端组件结构

```
SupportChatWidget.vue
  v-if="shouldRender"            (D2 三层守卫)
  ├─ SupportChatBubble.vue       (收起态 FAB)
  └─ SupportChatPanel.vue        (展开态)
      ├─ Header (title + close + clear-session)
      ├─ Body
      │   ├─ WelcomeArea         (首次/清空后显示，含 FAQ quickbar)
      │   └─ MessageTimeline     (user/assistant/system 三种气泡)
      ├─ ErrorBanner             (限流/网络错/key 失效时)
      └─ Footer
          ├─ TextInput           (Enter=send, Shift+Enter=newline)
          ├─ SendButton
          └─ FooterActions       ("提交工单" + "清空对话")
```

### D12. localStorage 持久化的边界

```
key:    support_chat_session_v1
value:  JSON {
          messages: [...],          // 完整对话历史 (UI 看的)
          updated_at: "ISO time"    // 用于"30 天未交互自动清空"
        }

写时机:  每次 user/assistant 消息完成 (assistant 流式收完最后 chunk 后 1 次)
读时机:  组件 mount 时
软上限:  100 条 message (超过时砍最早的，保留最近 100 条)
软上限:  30 天未更新 → 自动清空 (开机时检查 updated_at)
导出:    "提交工单" 按钮触发时调 store.exportAsMarkdown(), 写入同一 key (复用)
```

为什么用 `_v1` 后缀：将来 schema 升级时可以 graceful migrate（读不到 `_v2` 就读 `_v1` 转换）。

### D13. "提交工单"按钮的行为

```
点击时:
  1. 检查 support_ticket_enabled === true（来自 PublicSettings）
     false → 按钮被禁用，hint = "工单系统未启用"
  2. 已登录 → router.push('/support/tickets/new?from=chat&session=support_chat_session_v1')
     未登录 → router.push('/login?redirect=' + encodeURIComponent('/support/tickets/new?from=chat&session=support_chat_session_v1'))
  3. 工单新建页（add-support-ticket-system 已实现）会读 localStorage，
     把对话历史拼成 Markdown 填充到 content 草稿 + 同时塞 hidden chat_context
```

`session=support_chat_session_v1` 这个参数其实就是 localStorage 的 key，目前 widget 单实例所以 key 是固定的；保留 query 形态是为将来"多会话"扩展不破坏入参契约。

### D14. system prompt 模板

本 change 的 system prompt 由两部分拼接：

```
[硬编码框架]
你是 {{site_name}} 的客服助手。请只回答与 {{site_name}} 相关的问题（充值、API、模型、订单、账号等）。
如果用户问到与 {{site_name}} 无关的内容，礼貌引导回主题。
如果你不确定答案，建议用户提交工单（不要瞎编）。

[admin 配置 support_chat_system_prompt]
{{admin 在 textarea 里写的内容}}

[占位 hook for RAG]
{{rag_context}}      ← 本 change 始终为空字符串；add-support-knowledge-rag 会填充
```

**为什么硬编码框架**：项目里"不要瞎编"这种安全规则不能让 admin 删掉；admin 配置的 prompt 只是补充。

## Risks / Trade-offs

- **[客服 key 被薅干]** → 三层限流 + max_turns + max_tokens 兜底；admin 后台 key 余额可见。Mitigation：D5 / D7 已设计；admin 可在 key 用完时关 `support_chat_llm_enabled` 单独关 LLM，FAQ + 工单仍可用。
- **[localStorage 跨用户共用]** → 共享设备上"用户 A 的对话被用户 B 看到"。Mitigation：登出时 store.clearSession()；admin 可在 widget 加一个"切换账号会自动清空"的提示文案；不做更激进的清理（IP / cookie 关联）以免误伤。
- **[未登录但 anonymous_llm = true 时被刷]** → IP 限流 + 后端日志 + admin 可临时关 anonymous_llm 应急。Mitigation：默认就是关的，admin 知道开它有风险。
- **[硬编码 system prompt 与 admin 配置冲突]** → 例如 admin 写 "你可以回答任何问题"，硬编码写 "只回答 site 相关"。当 LLM 有歧义时听硬编码（顺序：硬编码框架在前，admin 在后；后写覆盖前 — 这是 LLM 通常行为）。**这其实是个反例 ——** Mitigation：把硬编码框架放在 system prompt **末尾**，覆盖 admin 输入；并在 admin 后台 textarea 上方明确说明"以下规则会被平台底线规则约束"。
- **[SSE 断流后前端状态不一致]** → 上游断流时通过 `event: error` 事件发给前端；前端把"未完整接收"的 assistant 消息标记为"中断"并提供"继续"按钮（M1 简化为"重试"）。
- **[路由排除 `/admin/*` 让 admin 调试浮窗费劲]** → admin 可以临时把这条排除规则去掉。Mitigation：tasks 里加一条 admin help 文案 "如需在 admin 路径调试，可在排除列表中删除 /admin/*"。
- **[流式响应的 token 计数与计费]** → 走自身 `/v1/chat/completions` 内部转发，已有 gateway 计费链路自动处理；本 change 不增加新计费路径。
- **[`max_request_tokens` 截断丢失重要早期消息]** → 截断算法保留 system + 最新 user，drop 最老的 assistant/user 对；drop 后在响应里加一个 `metadata.truncated_messages: N` 字段供前端显示提示（M1 可省）。

## Migration Plan

1. **依赖**：必须先合并 `add-support-ticket-system`（提供 `/support/tickets/new` 页 + `support_ticket_enabled` 设置）。
2. **后端先行**：
   - PublicSettings 新字段（向后兼容，老前端忽略）；
   - 新 API 端点上线（`support_chat_enabled = false` 时 SSE 端点返回 404，FAQ 端点返回空数组）；
   - 部署后端，但用户感知不到。
3. **前端跟进**：
   - widget + store + i18n + admin 设置 tab 一并发；
   - 部署后默认仍是关闭状态，不影响现网。
4. **首次启用**：
   - admin 在系统设置 → 客服里：(a) 选客服 API key (b) 填 system prompt (c) 维护 FAQ (d) 勾"启用浮窗" (e) 视情况勾"启用 LLM 应答"；
   - 全站浮窗即时出现。
5. **回滚**：
   - 关 `support_chat_enabled` → 浮窗消失；
   - 关 `support_chat_llm_enabled` → 浮窗仍在但 LLM 输入禁用（仅 FAQ + 工单入口）；
   - 代码 revert：单 commit 可回；schema 无破坏。

## Open Questions

- **是否要在 widget 上显示"AI 生成内容仅供参考"的免责声明？**——倾向：必须做，作为 panel footer 上的小字常驻；i18n key `support.chat.disclaimer`。
- **是否要支持 markdown 在 assistant 消息里渲染？**——倾向：是。用项目里如果已有的 markdown 组件复用；只支持粗体/斜体/链接/code block，不开 raw HTML。
- **是否要在 admin 后台显示"客服对话日志"？**——M1 不做（隐私 + 工程量）；如有需要走"用户主动提交工单时把对话拼进 chat_context"路径（已经有了）。后续可单独 propose。
