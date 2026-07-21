## Context

### 当前系统

sub2api 目前的实时通知能力分散且不通用：

- **工单通知**：`support_ticket_notification` 表 + `useTicketUnreadStore` 前端每 60s 轮询。表结构强绑定 `support_tickets` 外键；前端 `POST .../notifications/:id/read` 标记已读。
- **站内公告**：独立的 `announcements` 表 + `useAnnouncementsStore`，读游标语义与工单不同。
- **红点聚合**：`AnnouncementBell.vue` 两 Tab 布局，加和两个 store 的未读数。

新业务无法接入现有两条链路，只能新建表 + 新 store + 新轮询。

### 参考模型

主流成熟做法（Slack、Discord、Telegram Web 客户端）都用"单条 seq + 累积 ack"作为客户端最小心智负担。fan-out on write vs fan-out on read 是经典权衡。sub2api 决策：

- **单播用 fan-out on write**：本来就要 per-user 存一份（recipient 不同），落到 `direct_messages`。
- **广播用 fan-out on read**：发布 O(1)，读取时 targeting 过滤。避免 1 万用户广播时的 delivery 表爆炸。

同时采用三段组合保证语义正确：

- 存储严格一次：DB 唯一约束
- 推送至少一次：WS + REST catchup
- 消费恰好一次：客户端 `seen_seqs` 幂等 + 累积 ack

### 目标项目约束

- PostgreSQL SQL migrations 为 schema 事实源；Ent 自动迁移不是生产建表入口。
- 后端 Go + Gin + Wire；前端 Vue 3 + TS + pnpm。
- Redis 已是运行基础设施（session、feature flag 已在用）。
- 新模块集中在 `backend/internal/inbox/`。
- WebSocket 用 `gorilla/websocket`。
- 前端持久化用 `localStorage`。

## Goals / Non-Goals

### Goals

1. 存储严格一次（DB 唯一约束）
2. 推送至少一次（WS + catchup）
3. 消费恰好一次（客户端 `seen_seqs` 幂等 + 累积 ack）
4. 全局单调递增 seq，客户端只需一套算法
5. 单播 / 广播 使用不同表结构但对上层协议一致
6. 广播 O(1) 发布，不受用户数扩张影响
7. 广播支持属性目标（业务方注册 attribute provider）
8. 业务方 SDK 简单：一个 `PublishToUser` 一个 `PublishBroadcast`

### Non-Goals

1. 不做消息撤回原语
2. 不做多端并存（v1 全局单 WS）
3. 不做消息优先级 / 分类目录
4. 不做移动推送（APNs / FCM）
5. 不做已读跨端 UI 推送
6. 不做消息搜索
7. 不 backfill 存量通知历史

## Decisions

### D1. 两张独立表（单播 + 广播），无中间 delivery 表

**决策**：

```sql
direct_messages (
  seq          BIGINT PRIMARY KEY,        -- Redis 分配的全局 seq
  user_id      BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  namespace    VARCHAR(64) NOT NULL,
  dedup_key    VARCHAR(128) NOT NULL,
  payload      JSONB NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (user_id, namespace, dedup_key)   -- 幂等
);
CREATE INDEX ix_direct_user_seq ON direct_messages (user_id, seq);

broadcasts (
  seq          BIGINT PRIMARY KEY,        -- Redis 分配的全局 seq
  namespace    VARCHAR(64) NOT NULL,
  dedup_key    VARCHAR(128) NOT NULL,
  targeting    JSONB NOT NULL,
  payload      JSONB NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (namespace, dedup_key)            -- 幂等
);
CREATE INDEX ix_broadcasts_created ON broadcasts (created_at);

user_inbox_state (
  user_id     BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  acked_seq   BIGINT NOT NULL,             -- 无 default; 首次 catchup 时懒初始化
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**依据**：
- 单播和广播各自 fan-out 模型不同（write vs read），强融合成一张表反而复杂。
- 引入中间 `notification_deliveries` 表把广播扇出到每个用户会导致 1 万用户广播 = 1 万行 delivery，且发布路径变成 O(N) 而非 O(1)。前几轮讨论过被否决。

**替代方案**：
- 单表 envelope + delivery：见 proposal 前几版，已放弃。
- broadcasts 表内联 recipient_list：把匹配用户 ID 数组存进列，无法索引，查询困难。放弃。

### D2. 全局统一 seq，Redis 分配

**决策**：direct_messages.seq 与 broadcasts.seq 共用同一个 Redis 分配器；`seq = redis_time_ms * 2^20 | INCR_within_ms`。

**Lua 脚本**：
```lua
-- KEYS[1] = "seq:inbox:global"
-- returns int64 seq
local time = redis.call('TIME')
local ms = tonumber(time[1]) * 1000 + math.floor(tonumber(time[2]) / 1000)
local counter_key = KEYS[1] .. ':' .. ms
local n = redis.call('INCR', counter_key)
redis.call('PEXPIRE', counter_key, 2000)   -- 2 秒 TTL, 防内存增长
return ms * 1048576 + n                    -- ms << 20 | n
```

**性质**：
| 属性 | 保证 |
|---|---|
| 单调递增 | Redis 时钟单调 + 单毫秒内 INCR 单调 |
| 单毫秒并发上限 | 2^20 ≈ 100 万 |
| Redis 冷启动 | 时间戳保证新 seq >> 旧 seq |
| 数据丢失 | 从 1 开始 INCR，但时间戳保证新 seq 依然远高于历史 |
| 时钟回拨 <2s | counter_key TTL 2s，INCR 继续从上次值+1 |
| 时钟回拨 >2s | 可能重复分配 → DB PK 冲突 → 应用侧重试 |

**PK 冲突兜底重试**：
```go
for attempts := 0; attempts < 3; attempts++ {
    seq := allocSeqFromRedis(ctx)
    _, err := repo.InsertDirectMessage(ctx, seq, userID, ...)
    if !errors.Is(err, ErrDuplicateSeq) { return seq, err }
}
return 0, ErrSeqAllocExhausted   // 报警, 极罕见
```

**依据**：
- Redis TIME 拿的是服务端时钟，避免应用侧机器时钟不一致
- 不需要 DB seed / warmup，Redis 完全冷启动也能自愈（因为时间戳单调）
- Lua 脚本原子，无并发问题
- 单次 <1ms，不占 DB 行锁

**替代方案**：
- 纯 Redis INCR（无时间戳）：Redis 数据丢失后 seq 从 1 开始 → 与历史 direct_messages.seq 冲突。放弃。
- DB `user_inbox_state.next_seq` + 行锁分配：性能太低（每用户串行）。放弃。
- Snowflake 类分布式 ID：过度设计，需要 worker ID 分配等基础设施。放弃。

### D3. Acked_seq 懒初始化（新用户 / 迁移用户都从"now"开始）

**决策**：`user_inbox_state.acked_seq` 无 default，首次 catchup 时才 INSERT：

```go
func Catchup(ctx, userID, sinceClient) (Response, error) {
    state, err := repo.GetInboxState(ctx, userID)
    if errors.Is(err, sql.ErrNoRows) {
        freshSeq := allocSeqFromRedis(ctx)
        _ = repo.InsertInboxStateIfAbsent(ctx, userID, freshSeq)
        // ON CONFLICT (user_id) DO NOTHING; 并发时另一 goroutine 已初始化, 重读拿到实际值
        state, _ = repo.GetInboxState(ctx, userID)
    }
    effectiveSince := max(sinceClient, state.AckedSeq)
    ...
}
```

**依据**：
- 新用户从"注册后打开信箱那一刻"开始收消息，不会因为 30 天前有一条 `{__all__: true}` 广播就看到过期内容。
- 存量老用户升级后行为一致：首次 catchup 初始化到 `now` → 历史 `support_ticket_notification` 不进新信箱。
- 产品语义定为"信箱系统从上线时刻启用"。用户仍通过工单列表红点感知历史未读。
- 免除了 `joined_seq` 独立字段，`user_inbox_state` 只剩 3 列。

**代价**：
- 存量用户上线前 admin 发的广播、单播消息会被漏。**接受**（保留期 30 天内的少量数据不 backfill）。

### D4. 广播 fan-out on read

**决策**：
- **发布**：`INSERT INTO broadcasts VALUES ($seq, $ns, $dk, $targeting, $payload) ON CONFLICT DO NOTHING`，然后 Redis Pub/Sub 通知在线用户 → 各实例 Hub 遍历本地连接的用户 → 对每个用户求 `targeting.Match(user_attrs)` → 命中则 push。
- **读取（catchup）**：
  ```sql
  SELECT * FROM broadcasts
  WHERE seq > $effective_since
    AND created_at > now() - INTERVAL '30 days'
  ORDER BY seq
  LIMIT 200   -- 预取足够行, 应用层过滤后再返回 limit=100 给客户端
  ```
  应用层用 `Targeting.Match(user_attrs)` 二次过滤，然后与 direct_messages 合并按 seq 排序取前 N。

**依据**：
- 发布 O(1)，不因用户数扩张变慢。
- 广播行数低（估算日均 <100 条），30 天保留 <3000 行；扫描成本可控。
- Targeting JSON 表达式在 SQL 里求值太复杂，用 Go 内存求值简单。

**替代方案**：
- 发布时扇出（前几版方案）：1 万用户级广播 = 1 万 DB insert，且要处理扇出中断恢复。放弃。

### D5. Push 与 unacked_list 派生

**决策**：每条 WS push 携带派生的 `unacked` seq 列表：

```json
{
  "type": "notification",
  "seq": 1088,
  "scope": "direct" | "broadcast",
  "namespace": "support_ticket",
  "unacked": [1080, 1085, 1088],
  "truncated": false,
  "payload": {...},
  "created_at": "..."
}
```

**服务端派生**：
```sql
-- direct 部分
SELECT seq FROM direct_messages
WHERE user_id = $u AND seq > $acked_seq AND seq <= $current_seq
ORDER BY seq
-- broadcast 部分
SELECT seq, targeting FROM broadcasts
WHERE seq > $acked_seq AND seq <= $current_seq
  AND created_at > now() - INTERVAL '30 days'
ORDER BY seq
-- 广播部分需要在应用层用 targeting.Match(user_attrs) 过滤

-- 合并两路结果, 按 seq 排序, 取前 50 项;
-- 超过 50 则 truncated=true
```

**依据**：
- `unacked` 是 view 而非实体，不落表。
- LIMIT 50 + `truncated=true` 防离线用户上线时 payload 爆炸；客户端见 `truncated` 触发 catchup。

### D6. 累积 Ack + 客户端连续段推进

**决策**：`user_inbox_state.acked_seq` 单调水位；客户端只在**连续无洞**的 seq 段末端调 `ack(n)`。

服务端：
```sql
INSERT INTO user_inbox_state (user_id, acked_seq)
VALUES ($u, $n)
ON CONFLICT (user_id) DO UPDATE
  SET acked_seq = GREATEST(user_inbox_state.acked_seq, EXCLUDED.acked_seq),
      updated_at = now()
```

Ack 与 catchup 都是懒 upsert，容错。

客户端算法（严格描述见 spec）：
```
从 local_ack_seq+1 起在 seen_seqs 中寻找最长连续段末端 n
若 n > local_ack_seq: 排入 pending_ack, 300ms defer 后统一发送
```

**依据**：
- 单水位存储简单，SQL 幂等。
- "连续无洞才推进"由客户端算法保证；服务端不做校验。
- 累积 ack 天然支持多条消息一次性 ack。

### D7. 客户端持久化 `local_ack_seq` + `seen_seqs`

**决策**：客户端在 `localStorage` 持久化两个键：

- `inbox:{user_id}:ack_seq` — 已 ack 水位
- `inbox:{user_id}:seen_seqs` — 已 render 但未 ack 的 seq 列表

**写入时机**：
- `render(seq)` 后立即 persist `seen_seqs`
- Server ack `200` 后 persist `local_ack_seq` + 裁剪 `seen_seqs`

**依据（用户提出的关键 case）**：
- 若不持久化 `seen_seqs`，"已 render 42 未 ack + 页面刷新" 会导致 catchup 重返 42 且被再次 render，重复消费。
- 持久化后：刷新时识别"42 已 render 过"跳过 render 但参与 ack 推进。

### D8. Ack 防抖（客户端）

**决策**：客户端 ack 使用 300ms defer 定时器，多条 push 到达时只重排目标 seq，实际 RPC 300ms 后打一次。

```
pendingAck = null
ackTimer = null

trySchedulAck():
  n = 从 local_ack_seq+1 起找 seen_seqs 中最长连续段末端
  if n <= local_ack_seq: return
  pendingAck = n
  if not ackTimer:
    ackTimer = setTimeout(300ms, flushAck)

flushAck():
  n = pendingAck; pendingAck = null; ackTimer = null
  await server.ack(n)
  local_ack_seq = n
  persist(local_ack_seq, 裁剪 seen_seqs)
```

**特殊时机强制 flush**：
- `visibilitychange → hidden`（用户切走标签页）
- `beforeunload`（页面关闭前）→ 用 `navigator.sendBeacon`
- WS 断线前

**依据**：
- 100 条密集消息在 300ms 内到达 → 只发 1 次 `ack(max_seq)`
- 累积 ack 语义天然聚合，服务端不需要改
- 稀疏消息延迟可接受
- **可调参**：defer 时间 100~1000ms 权衡实时性 vs 网络请求数

### D9. 冷启动 `bootstrapping` 状态

**决策**：客户端引入 `bootstrapping` 布尔状态。

```
启动:
  bootstrapping = true
  local_ack_seq, seen_seqs = load(localStorage)
  ws.connect()
  await catchup(since=local_ack_seq)
  bootstrapping = false
  process buffered_pushes

处理 push:
  if bootstrapping:
    buffered_pushes.push(msg)   -- 只入队, 不 render, 不响应 truncated
    return
  ...稳态逻辑
```

**依据（用户提出的场景）**：
- WS 建立后服务端可能立即 push 一条 `truncated=true` 消息
- 同时客户端也在冷启动主动 catchup
- 若不做保护：`truncated` 触发第二次 catchup，与冷启动 catchup 冗余

**性质**：
- 冷启动只有一次网络往返
- 稳态 `truncated=true` 才真正触发拉取
- 队列在冷启动短时（<1s）不会爆炸

### D10. 每用户全局单一 WS（新连接踢旧连接）

**决策**：
- 服务端 `InboxHub` 维护 `map[user_id] *wsConn`
- 新连接注册时踢旧（本地 + 跨实例经 Redis Pub/Sub `sub2api:inbox:kick`）
- 踢出协议：向旧连接发 `{type:"kicked", reason:"opened_elsewhere", client_type:"web"}` + close code 4001
- 客户端见 4001 展示"您已在 XX 端打开"遮罩，**不自动重连**
- 提供"在此继续"按钮 → 主动重连反过来踢当前活跃端

**client_type 语义**：
- WS 握手时通过 Subprotocol 携带（`Sec-WebSocket-Protocol: inbox.v1, bearer.<token>, ct.web`）
- 白名单 `{web, ios, android, desktop}`，其他归一为 `unknown`
- **只用于踢出提示的友好文案**；v1 registry key 是 `user_id`（不隔离 client_type）
- v2 若切多端并存只需 registry key → `(user_id, client_type)`，协议不变

**依据**：
- 单例避免多 tab 重复 render + `localStorage` 竞争
- Redis Pub/Sub 是已有基础设施
- v1 只做 UX 提示，实现简单

**边界**：
- 网络异常（close code 1006）走标准指数退避重连，不显示 kicked 遮罩
- 快速刷新（旧连接 close 事件还没到）：新连接踢的是自己上一个连接，无害

### D11. Publisher SDK 与业务集成

**Go 接口**：
```go
type Publisher interface {
    PublishToUser(ctx context.Context,
                  namespace, dedupKey string,
                  recipientID int64,
                  payload []byte) (seq int64, err error)

    PublishBroadcast(ctx context.Context,
                     namespace, dedupKey string,
                     targeting Targeting,
                     payload []byte) (seq int64, err error)
}
```

**业务方接入示例**：
```go
// 用户创建工单 → 通知所有 admin
payload, _ := json.Marshal(TicketEventPayload{
    EventType:  "ticket_created",
    TicketID:   ticket.ID,
    Title:      ticket.Title,
    Excerpt:    makeExcerpt(ticket.Content),
    LinkAdmin:  fmt.Sprintf("/admin/support/tickets/%d", ticket.ID),
})
_, err := publisher.PublishBroadcast(ctx,
    "support_ticket",
    fmt.Sprintf("created:%d", ticket.ID),
    inbox.Targeting{Filter: map[string]any{"role": "admin"}},
    payload,
)
```

**Publish 失败 MUST 不阻塞业务主流程**：`log.Warn` 后继续。

### D12. Targeting 白名单表达式

**决策**：`Targeting.Filter` 是 JSON 表达式，服务端只支持有限 op：

| op | 语法 | 示例 |
|----|------|------|
| equals | `{"key": "value"}` 或 `{"key": {"equals": "value"}}` | `{"role": "admin"}` |
| in | `{"key": {"in": [v1, v2, ...]}}` | `{"role": {"in": ["admin","staff"]}}` |
| all_users | `{"__all__": true}` | `{"__all__": true}` |
| and | `{"and": [<expr>, <expr>, ...]}` | `{"and":[{"role":"admin"},{"country":"CN"}]}` |
| or | `{"or": [<expr>, <expr>, ...]}` | `{"or":[{"role":"admin"},{"is_vip":true}]}` |

其他 op 返回 `400 unsupported_targeting_op`。

**求值路径**：
- `AttributeProvider.Fetch(ctx, userID) → map[string]any`
- 递归匹配 `Filter`，命中则 targeting 匹配

**默认 provider** `UserBasicProvider`：读 users 表暴露 `id, role, country, plan_tier, created_at`。

**注意 fan-out on read 不需要 QueryUsers**：广播不做发布时扇出，AttributeProvider 只需要 `Fetch(userID)`。这是与前几版方案的关键简化。

### D13. Namespace 治理与 payload 约束

- `namespace` 白名单：`inbox.RegisterNamespace(name, metadata)` 集中注册；未注册 publish 返回 400
- `dedupKey` 正则：`^[a-zA-Z0-9:._-]{1,128}$`
- `payload` JSONB 硬限 8 KiB；必须是合法 JSON object（非 null，非 array）
- payload 应包含足以让前端渲染的所有字段（title, excerpt, link, event_type...）；不推荐"引用 ID 让前端二次拉"

## Risks / Trade-offs

| 风险 | 影响 | 缓解 |
|------|------|------|
| Redis 时钟回拨 >2s | seq 分配可能与已有值冲突 | DB PK 兜底重试 3 次；报警 |
| Redis 完全宕机 | seq 无法分配，publish 全部失败 | Redis 是已有基础设施；宕机全站受影响；降级到 fail-open 日志 |
| 广播 catchup 扫全表 | 30 天广播行数偏多时 | 时间窗 + `(created_at)` 索引 + `SELECT` 侧 LIMIT 200 |
| 广播 targeting 在应用层求值 | 需 CPU 遍历判断 | 30 天 <3000 行，忽略级；未来量大可加缓存 |
| 客户端 seen_seqs bug | 跳过 seq 导致漏消费 | 集成测试 + 服务端 unacked 提示自愈 |
| localStorage 多 tab 竞争 | v1 单 WS 已避免 | v1 接受，v2 引入 web-locks |
| Redis Pub/Sub 丢失 | 跨实例踢连接漏 / 广播不到达 | 广播不到达可由 catchup 兜底；踢连接漏无致命影响（旧连接自然超时） |
| payload 逃逸敏感字段 | 业务方误放 secret | 代码审查 + 日志脱敏 + payload 白名单约定 |
| 不 backfill 历史通知 | 老用户升级后信箱空 | 保留工单红点（`support_ticket_reads`）；产品文档说明 |

## Migration Plan

### 阶段 1：模块骨架 + 数据基础（PR-1）

- 建立 `backend/internal/inbox/` 目录
- SQL migration：`direct_messages`, `broadcasts`, `user_inbox_state`
- Repository 层 + 单元测试

### 阶段 2：Redis seq 分配（PR-2）

- Lua 脚本 + Go 封装
- PK 冲突重试
- 单元测试：并发分配、时钟回拨模拟、Redis 冷启动

### 阶段 3：Publisher SDK + REST（PR-3）

- `PublishToUser` / `PublishBroadcast`
- REST：`GET /api/v1/inbox/messages`, `POST /api/v1/inbox/ack`, `GET /api/v1/inbox/unacked-count`
- 管理端：`POST /api/admin/inbox/broadcast`
- AttributeProvider + `UserBasicProvider`

### 阶段 4：WebSocket 推送（PR-4）

- `InboxHub` + `gorilla/websocket`
- 踢出协议 + Redis Pub/Sub 跨实例
- 广播 Pub/Sub 通道
- 单元测试 + 端到端集成测试（丢包、乱序、断线重连、kicked）

### 阶段 5：前端集成（PR-5）

- `useInboxStore` + WS 客户端
- `bootstrapping` 状态
- `localStorage` 持久化
- ack 300ms 防抖

### 阶段 6：ticket-notifications 迁移（PR-6）

- 工单事件走信箱发布
- 前端 Ticket Tab 改用信箱 store
- `useTicketUnreadStore` 只保留 `unreadCount`
- 旧 REST 返回 410

### 阶段 7：清理与观察（后续 release）

- 观察 30 天
- 决定是否 drop `support_ticket_notification`
- 删除 410 兼容端点

## Resolved Decisions

以下项在 propose 阶段已拍板，作为实施约束记录：

- **广播 catchup 预取策略**：服务端预取 `2 × client_limit` 行，应用层过滤 targeting 后合并 direct + broadcast 按 seq 排序，取前 `client_limit` 返回；若合并结果不足且候选未耗尽则 `has_more=true`，客户端可继续 catchup。**不做 loop 拉更多**（保持单次 catchup 响应时间上限可控）。
- **管理员手动删除信箱消息**：v1 不提供 UI；30 天硬删清理 job 承担唯一清理路径。若需要紧急撤销广播，管理员发一条新 broadcast 表达"撤回"，前端按业务约定处理。
- **数据清理 job 抢主 lock**：Redis key `sub2api:inbox:cleanup:lock`，TTL 300s（覆盖单次 job 最大预计耗时），SETNX 抢主 + `pexpire`；job 完成后主动 DEL 释放。所有实例每小时尝试抢一次；未抢到的实例 no-op。
- **kicked reason**：v1 只使用 `opened_elsewhere` 一种；客户端根据 `client_type` 字段自行渲染友好文案（例如 web / iOS App / Android App / 桌面客户端 / 未知端）。未来若引入其他踢出场景（如强制下线、账号封禁）再新增 reason 枚举。
