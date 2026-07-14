# Design — add-support-chat-transcript-log

## 背景与约束

客服浮窗当前是无状态透传。关键既有事实（已从代码确认）：

- 浏览器 `supportChat.ts` 已生成并持久化 `session_id`（`makeSessionId()`，`clearSession()` 重置 = "新开一段对话"），**每次 `POST /support/chat` 的 body 都带 `session_id`**。
- 后端 `SupportChatRequest.SessionID` 已解析，但仅注释为"日志/审计"，未落库、未使用。
- `streamSSEFromUpstream(c, upstream)` 逐行读取上游 `data: ...\n\n` 分片、原样 `Write` + `Flush`，**不解析内容**。
- repository 层用 **ent ORM**（`support_ticket_repo.go` 是样板），migration 是**裸 SQL**（`150_add_support_tickets.sql`），新表需 SQL migration + ent schema + `go generate`。

因此本设计的两个技术核心：**(A) 如何在不改协议、不加延迟的前提下抓到流式回包**，**(B) 落库点必须覆盖 handler 的每一个返回分支**。

---

## D1. 数据模型：会话头 + 消息行（方案 B）

```
support_chat_conversations                support_chat_messages
┌───────────────────────────┐            ┌──────────────────────────────┐
│ id            BIGSERIAL PK │            │ id             BIGSERIAL PK   │
│ session_id    VARCHAR UNIQ │◀───1:N────│ conversation_id BIGINT FK     │
│ user_id       BIGINT NULL  │            │ role           VARCHAR(16)    │  user|assistant
│ client_ip     VARCHAR(64)  │            │ content        TEXT           │
│ turn_count    INT          │            │ status         VARCHAR(24)NULL│  仅 assistant 行有值
│ last_status   VARCHAR(24)  │            │ error_message  TEXT NULL      │
│ first_at      TIMESTAMPTZ  │            │ model          VARCHAR(128)NULL│
│ last_at       TIMESTAMPTZ  │            │ latency_ms     INT NULL       │
│ created_at    TIMESTAMPTZ  │            │ created_at     TIMESTAMPTZ    │
│ updated_at    TIMESTAMPTZ  │            └──────────────────────────────┘
└───────────────────────────┘
```

**为什么 `session_id` 做唯一业务键而不是 PK**：`session_id` 是客户端生成的字符串（不可信长度/字符集），用自增 `id` 做 PK + `session_id UNIQUE` 索引，既得到稳定内部主键，又能靠唯一约束做幂等 upsert。

**一轮问答 = 两行 message**：user 行（role=user，无 status）+ assistant 行（role=assistant，带 status/error/model/latency）。这样即使 assistant 回包失败（如 upstream_auth），user 行仍留痕——能看到"用户问了什么但没答上"。

**匿名对话**：`user_id` 可空（`BIGINT NULL`）。登录用户填 `user_id` + `client_ip`；匿名只填 `client_ip`。与 `support_ticket_replies.author_id ON DELETE SET NULL` 语义一致——用户注销后对话保留但脱钩。

**索引**：
- `support_chat_conversations (session_id)` UNIQUE — upsert 用。
- `support_chat_conversations (last_status, last_at DESC)` — admin 按状态过滤 + 时间倒序。
- `support_chat_conversations (user_id, last_at DESC)` — admin 按用户查。
- `support_chat_messages (conversation_id, created_at)` — 详情按时间取整段。

---

## D2. 落库粒度：为什么不是"一问一答一行"

同一 `session_id` 的多轮问答归并到一个会话头，`turn_count` 递增、`last_at` / `last_status` 刷新。理由：

- 管理员点开"客服对话记录"期望看到**一整段对话**（用户可能连问 5 轮），而非 5 条互不相干的记录。
- 客户端每次发的是**完整历史** `messages`，若一问一答存一行，历史会重复存 N 次。归并到会话 + 只追加"本轮新增的 user + assistant"避免冗余。

**"本轮新增"的判定**：客户端发来的 `messages` 末尾恒为最新 user（`truncateChatMessages` 保证）。落库时只取**最后一条 user** 作为本轮 user 行；assistant 行来自本次 tee 累积的回包。历史中更早的消息不重复落库（它们在之前的轮次已写过）。

---

## D3. 回包捕获：tee 累积（技术核心 A）

`streamSSEFromUpstream` 改造为在透传的同时累积助手文本。**先写 client、再累积**，保证透传延迟不受影响：

```
for each line from upstream:
    Write(line) → client        # 原有行为，延迟不变
    Flush()
    if line 以 "data: " 开头 且非 "[DONE]":
        parse JSON → choices[0].delta.content
        accumulator.WriteString(content)   # 旁路累积
    if line == "data: [DONE]":
        doneSeen = true
返回 (accumulatedText, doneSeen, readErr)
```

改造后签名从 `func streamSSEFromUpstream(c, upstream)` 变为返回累积结果：

```go
type streamResult struct {
    Text     string   // 累积的 assistant 回复
    DoneSeen bool     // 是否见到 [DONE]（正常收尾）
    Err      error    // 读取上游中断错误（含 client 断开导致的 ctx cancel）
}
func streamSSEFromUpstream(c *gin.Context, upstream io.Reader) streamResult
```

**解析容错**：delta 解析失败（非预期 JSON / 缺字段）时 **silent skip 该分片**，不影响透传、不中断累积。累积不到内容不是错误（可能是纯 function_call 之类），status 仍按流是否正常收尾判定。

**client 断开检测**：`c.Writer.Write` 返回 error（客户端断开）时，`Err` 非空且 `DoneSeen=false` → 判定 `interrupted`，用已累积的部分文本落库。

---

## D4. 落库点：覆盖每个返回分支（技术核心 B）

`PostChat` 有多个 `return`，落库必须在**每一个**上覆盖，且区分"打到上游前"与"打到上游后"：

```
PostChat 分支                              status          落库内容
──────────────────────────────────────────────────────────────────────
enabled=false / llm_enabled=false          （不落库）      浮窗没开，直接 404，无对话语义
creds 缺失 (base_url/api_key 空)            config_error    user 行 + 空 assistant 行(带 error)
未登录且 !anonymous_llm → 401               （不落库）      连消息都没发，无 user 内容
限流命中 → 429                              rate_limited    user 行 + 空 assistant 行(带 error)
参数非法 (messages 空 / bind 失败)          （不落库）      无有效 user 内容
上游 401 → SSE error 帧                     upstream_auth   user 行 + 空 assistant 行(带 error)
上游非200 → 502                             upstream_error  user 行 + 空 assistant 行(带 error)
流正常收尾 ([DONE])                         success         user 行 + assistant 行(累积文本)
流中途读取出错 / client 断开                interrupted     user 行 + assistant 行(部分文本+error)
```

**取舍说明**：
- `config_error` / `rate_limited` **要落库**（用户已提交问题，只是系统侧挡了）——按需求"记录全部"。但注意此时 `req` 已解析成功才能拿到 user 内容；限流分支在 bind **之前**，需**调整顺序**：把限流检查移到 bind 之后，或在限流分支单独轻量 bind 出 `session_id + 最后一条 user`。设计选**后者**（限流分支单独尝试 bind，失败则退化为不落库），避免为落库改动限流的既有安全顺序。
- `enabled=false` / 401(未登录) / bind 失败 **不落库**——这些分支要么浮窗根本没开、要么没有有效用户内容，落库只会产生噪声空行。

**落库时机**：成功/中断路径在 `streamSSEFromUpstream` 返回后（stream 已终结）用 `defer` 或顺序调用落库；早退路径在各自 `return` 前调用同一个落库 helper。**落库失败只 `slog.Warn`，绝不影响给用户的响应**（审计是旁路，不能拖垮主流程）。

**latency_ms**：记录从 `PostChat` 进入到 stream 收尾的耗时（对早退分支可为 NULL 或到 return 的耗时）。

---

## D5. Upsert 幂等与并发

`UpsertConversation(session_id, ...)` 用 ent 的 `sql/upsert` feature（generate.go 已启用 `--feature sql/upsert`）：

```
INSERT INTO support_chat_conversations (session_id, user_id, client_ip, turn_count, ...)
VALUES (...)
ON CONFLICT (session_id) DO UPDATE
  SET turn_count = support_chat_conversations.turn_count + 1,
      last_status = EXCLUDED.last_status,
      last_at     = EXCLUDED.last_at,
      user_id     = COALESCE(support_chat_conversations.user_id, EXCLUDED.user_id)
```

**并发**：同一 session 串行发问（浮窗 UI 一次一问，前一轮 streaming 结束才能发下一轮），并发 upsert 概率极低；即便发生，`ON CONFLICT` 保证不重复建会话。`turn_count` 的 `+1` 走原子 SQL 表达式，无读改写竞态。

**事务边界**：一轮落库 = upsert 会话 + append user 行 + append assistant 行，包在一个 ent 事务里（`clientFromContext` 已支持事务透传），保证"要么整轮留痕、要么整轮不留"，不会出现有 user 行没会话头的悬挂。

---

## D6. 管理端只读接口

```
GET /api/v1/admin/support/chat/conversations
    ?page=&page_size=&status=&user_id=&ip=&q=&from=&to=
    → { items: [{id, session_id, user_id, client_ip, turn_count,
                 last_status, first_at, last_at}], total, page, page_size }
    列表不返回消息正文（同工单 List 不返回 chat_context 的策略）。

GET /api/v1/admin/support/chat/conversations/:id
    → { conversation: {...}, messages: [{role, content, status,
        error_message, model, latency_ms, created_at}, ...] }
    详情返回整段消息时间线，按 created_at 升序。
```

- `q` 关键词走 message content 的 `ILIKE %q%`（ent `ContainsFold`，参数化，防注入）。
- `status` 过滤命中 `last_status`（会话级）。
- 挂在既有 `/api/v1/admin/support` 子组下，`AdminAuthMiddleware` 已覆盖鉴权，handler 不再校验权限（同工单 admin handler）。

---

## D7. 菜单 gating

侧边栏"客服对话记录"入口的可见性**跟随 `support_chat_enabled`**（public setting，前端已注入 `cachedPublicSettings.support_chat_enabled`）——与工单菜单跟随 `support_ticket_enabled` 同款。不新增 `support_chat_log_enabled` 独立开关，减少设置项。后端 admin 路由**不卡 feature flag**（管理员可随时查存量记录，与工单 admin 路由策略一致）。

---

## D8. 隐私与留存

- **匿名**：`user_id` 可空，匿名对话只存 IP。
- **content 上限**：单条 message content 落库前截断到 `50000` 字符（与工单 `chat_context` 上限对齐；`SupportChatMaxRequestTokens` 本身已限制入站体量，此为二次防御）。
- **留存**：M1 永久保留。未来若需合规清理，挂载点为一个 `support_chat_log_retention_days` setting + 复用既有 cron 基建（参考 `support_doc_indexer_cron.go`）的定时 DELETE。本 change 不实现，仅在此登记。
- **不脱敏正文**：客服对话正文按原文存（审计需要）；admin 端已受 `AdminAuthMiddleware` 保护。

---

## D9. 被否决的备选

| 备选 | 为何否决 |
|------|---------|
| 一问一答存一行（方案 A） | 管理端看不到"整段对话"；客户端每次发全量历史会导致重复落库 |
| 前端在 `onDone` 后单独 POST 回包落库 | 多一次往返；client 断开时拿不到回包；前端可篡改审计数据 |
| 只存二元 success/failed | 用户明确要完整分类，无法定位失败在鉴权/上游/中断哪一步 |
| 在 service 层再包一层 SSE 代理做落库 | handler 已持有全部上下文，另起一层徒增复杂度与延迟 |
| 用消息表自增 id 关联而非 session_id 唯一键 | 无法幂等归并同一会话的多轮问答 |
