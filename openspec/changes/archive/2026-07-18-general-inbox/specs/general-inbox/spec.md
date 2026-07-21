## ADDED Requirements

### Requirement: 两张独立表分离单播与广播

系统 SHALL 使用两张独立表分别承载单播和广播消息：

- `direct_messages` — 每用户每消息一行（单播 fan-out on write）
- `broadcasts` — 全局一份（广播 fan-out on read）

不引入中间 delivery 表。

`direct_messages` MUST 至少包含以下字段：

```
seq         BIGINT PRIMARY KEY           -- Redis 分配的全局 seq
user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE
namespace   VARCHAR(64)  NOT NULL
dedup_key   VARCHAR(128) NOT NULL
payload     JSONB        NOT NULL         -- octet_length(payload::text) <= 8192
created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
UNIQUE (user_id, namespace, dedup_key)     -- 单播幂等
```

`broadcasts` MUST 至少包含以下字段：

```
seq         BIGINT PRIMARY KEY           -- 与 direct_messages 同一 seq 数轴
namespace   VARCHAR(64)  NOT NULL
dedup_key   VARCHAR(128) NOT NULL
targeting   JSONB        NOT NULL         -- 属性表达式
payload     JSONB        NOT NULL         -- octet_length(payload::text) <= 8192
created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
UNIQUE (namespace, dedup_key)              -- 广播幂等
```

`user_inbox_state` MUST 至少包含以下字段：

```
user_id     BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE
acked_seq   BIGINT NOT NULL                -- 无 default; 首次 catchup 时懒初始化
updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
```

#### Scenario: 单播幂等

- **GIVEN** namespace=`support_ticket`, dedup_key=`created:42`, recipient_id=100
- **WHEN** 业务方连续调用 `PublishToUser` 两次
- **THEN** `direct_messages` MUST 只存在一行，第二次调用返回与第一次相同的 seq

#### Scenario: 广播幂等

- **GIVEN** namespace=`system_announcement`, dedup_key=`v1.5-release`, targeting=`{all_users:true}`
- **WHEN** 业务方连续调用 `PublishBroadcast` 两次
- **THEN** `broadcasts` MUST 只存在一行

#### Scenario: 单播与广播 dedup_key 相同不冲突

- **GIVEN** 已存在 direct_messages 行 `namespace=x, dedup_key=k, user_id=100`
- **WHEN** publish `broadcast namespace=x, dedup_key=k, targeting=...`
- **THEN** 新 broadcast 行 MUST 被创建（两张表隔离）

#### Scenario: payload 超限拒绝

- **WHEN** publish 的 payload JSON 字节数超过 8192
- **THEN** 系统 MUST 返回 `400 payload_too_large`，不写入任何行

### Requirement: Redis 分配的全局单调递增 seq

系统 SHALL 使用 Redis Lua 脚本原子分配 seq；`direct_messages.seq` 与 `broadcasts.seq` 共用同一个数轴。

Seq 结构 MUST 为：`seq = redis_time_ms * 2^20 + INCR_within_ms`，其中：

- `redis_time_ms` 由 Redis `TIME` 命令返回的服务器时钟（毫秒）
- `INCR_within_ms` 是同一毫秒内的自增序号，最大 2^20 - 1

Redis Lua 脚本 MUST 满足：

- 使用 `counter_key = "seq:inbox:global:{ms}"` per-ms 分片
- `PEXPIRE counter_key 2000` 防止 Redis 内存无限增长
- 返回 `int64` 类型 seq

应用侧调用 MUST 实现 PK 冲突重试：分配 seq → INSERT `ON CONFLICT DO NOTHING`；若 PK 冲突（表示 seq 已存在），重新分配再试，最多 3 次；3 次都冲突返回 `seq_alloc_exhausted`（应触发告警）。

Redis 不可用时 `SeqAllocator.Alloc` MUST 返回 `redis_unavailable` 错误；publish 相应返回错误（业务侧 fail-open）。

#### Scenario: 同毫秒内并发分配

- **WHEN** 同一毫秒内并发调用 `SeqAllocator.Alloc` 1000 次
- **THEN** 返回的 1000 个 seq MUST 全部单调递增且无重复；`INCR_within_ms` 分量 MUST 是 `1..1000`

#### Scenario: 跨毫秒分配保持单调

- **GIVEN** `t=T` 分配了 seq=S
- **WHEN** `t=T+1ms` 分配一个新 seq
- **THEN** 新 seq MUST > S

#### Scenario: Redis 冷启动后 seq 仍单调

- **GIVEN** 历史已分配到 seq = 1048576000000
- **WHEN** Redis 完全宕机数据丢失后重启，在新的毫秒时刻分配 seq
- **THEN** 新 seq MUST > 1048576000000（因时间戳单调）

#### Scenario: Redis 不可用返回明确错误

- **WHEN** Redis 完全不可达
- **THEN** `SeqAllocator.Alloc` MUST 返回 `redis_unavailable`；`PublishToUser` / `PublishBroadcast` MUST 返回同错误码；业务方 SHOULD `log.Warn` 后继续主流程

### Requirement: user_inbox_state 首次访问懒初始化

`user_inbox_state.acked_seq` 无 DB default 值。首次 catchup 时若无行，服务端 MUST 分配 `fresh_seq = SeqAllocator.Alloc()` 作为初始水位，通过 `INSERT ... ON CONFLICT (user_id) DO NOTHING` 落行；并发时另一 goroutine 若已初始化，MUST 重读拿实际值。

Ack 端点 MUST 也使用懒 upsert：`INSERT ... ON CONFLICT (user_id) DO UPDATE SET acked_seq = GREATEST(existing, EXCLUDED)`。

初始化后，`acked_seq` MUST 单调抬升，MUST NOT 被后续操作降回。

#### Scenario: 新用户首次 catchup 初始化

- **GIVEN** 用户 U 是新注册用户，`user_inbox_state` 无行
- **WHEN** U 首次调用 `GET /api/v1/inbox/messages?since_seq=0&limit=50`
- **THEN** 服务端 MUST 分配 fresh_seq、INSERT `user_inbox_state (U, fresh_seq)`；响应 MUST 返回 `acked_seq = fresh_seq`；`messages` 数组 MUST 为空（因所有历史消息 seq ≤ fresh_seq）

#### Scenario: 存量老用户升级后首次开信箱是空的

- **GIVEN** 系统上线前已有用户 U，`support_ticket_notification` 中有历史通知
- **WHEN** U 升级后首次调用 catchup
- **THEN** 同样触发懒初始化到 `fresh_seq(now)`；`messages` MUST 为空；历史 `support_ticket_notification` MUST NOT 出现在响应中

#### Scenario: 并发首次访问不重复初始化

- **GIVEN** 用户 U 无 `user_inbox_state` 行
- **WHEN** 两个 goroutine 并发触发首次 catchup
- **THEN** 只有一行被创建（`ON CONFLICT DO NOTHING`），两个 goroutine 最终读到相同的 `acked_seq`

### Requirement: 累积 Ack 语义

系统 SHALL 通过 `user_inbox_state.acked_seq` 表达"已确认接收到 seq ≤ acked_seq 的所有消息"。

Ack 端点 `POST /api/v1/inbox/ack {seq: n}` 的语义 MUST 是：

```sql
INSERT INTO user_inbox_state (user_id, acked_seq)
VALUES ($u, $n)
ON CONFLICT (user_id) DO UPDATE
  SET acked_seq = GREATEST(user_inbox_state.acked_seq, EXCLUDED.acked_seq),
      updated_at = now()
```

Ack MUST 幂等：相同或更小的 `n` 多次调用返回 `200` 且不改状态。

客户端 MUST 保证只在**连续无洞**的 seq 段末端调 ack —— 即客户端本地已 render `local_ack_seq+1, +2, ..., n` 每一个 seq 时才能 `ack(n)`。此约束由客户端算法保证，服务端不做严格校验。

服务端 SHOULD 对 `seq < 1` 做基础范围校验，返回 `400 ack_out_of_range`。

#### Scenario: Ack 单调抬升

- **GIVEN** `acked_seq=1000`
- **WHEN** 客户端调 `POST /inbox/ack {seq: 1500}`
- **THEN** `acked_seq` 变为 1500

#### Scenario: Ack 更小值幂等

- **GIVEN** `acked_seq=2000`
- **WHEN** 客户端调 `POST /inbox/ack {seq: 1500}`
- **THEN** `acked_seq` 保持 2000，返回 `200`

#### Scenario: Ack 非法值拒绝

- **WHEN** 客户端调 `POST /inbox/ack {seq: -1}` 或 `{seq: 0}`
- **THEN** 返回 `400 ack_out_of_range`

### Requirement: 广播 fan-out on read

广播 MUST 采用 fan-out on read 模型：

- **发布路径 O(1)**：`INSERT INTO broadcasts` + `Redis PUBLISH sub2api:inbox:broadcast {seq}`；不遍历用户列表，不写 delivery 行。
- **推送路径**：各实例 Hub 订阅 `sub2api:inbox:broadcast`；收到事件后遍历本地已连接的所有用户，对每个用户执行 `AttributeProvider.Fetch(userID)` → `Targeting.Match(attrs)`；命中则组装 PushMessage 并发送到该用户的 WS 连接。
- **拉取路径（catchup）**：
  1. `SELECT * FROM broadcasts WHERE seq > $effective_since AND created_at > now() - '30 days' ORDER BY seq LIMIT 2*client_limit`
  2. 应用层用 `Targeting.Match(user_attrs)` 二次过滤
  3. 与 direct_messages 结果合并按 seq 排序取前 `client_limit`

广播 targeting 求值 MUST 只发生在推送/读取阶段，MUST NOT 在发布时反查用户列表。

#### Scenario: 发布广播 O(1)

- **GIVEN** targeting 匹配 100 万用户
- **WHEN** publish broadcast
- **THEN** 数据库写入 MUST 只有 1 行（`broadcasts`），Pub/Sub 消息只发 1 条；发布路径耗时 MUST < 50ms

#### Scenario: 离线用户上线后 catchup 收到广播

- **GIVEN** 广播 B (targeting=`{role:"admin"}`) 已发布，用户 U（role=admin）当时离线
- **WHEN** U 上线后调用 catchup
- **THEN** B MUST 出现在 messages 数组中

#### Scenario: 属性不匹配的用户不收到广播

- **GIVEN** 广播 B (targeting=`{role:"admin"}`) 已发布，用户 U (role=user) 在线
- **WHEN** Hub 处理 broadcast Pub/Sub 事件
- **THEN** U 的 WS 连接 MUST NOT 收到 B；U 后续 catchup MUST NOT 包含 B

#### Scenario: 属性变更后能补历史广播

- **GIVEN** 广播 B (targeting=`{role:"admin"}`) 已发布；用户 U 当时 role=user，未收到；随后 U 升级为 admin
- **WHEN** U 调用 catchup
- **THEN** 服务端应用层过滤时 `Targeting.Match(current_attrs)` 返回 true，B MUST 出现在响应中（B 仍在 30 天保留期内）

### Requirement: WebSocket 推送与 unacked 派生

系统 SHALL 通过 WebSocket 主推消息。每条推送 message MUST 包含 `unacked` 列表：

`unacked` 服务端派生逻辑：
1. `SELECT seq FROM direct_messages WHERE user_id=$u AND seq > $acked_seq AND seq <= $current_seq LIMIT 50`
2. `SELECT seq, targeting FROM broadcasts WHERE seq > $acked_seq AND seq <= $current_seq AND created_at > now()-'30d' LIMIT 50`
3. 广播结果集应用层用 `Targeting.Match(user_attrs)` 过滤
4. 两路合并按 seq 排序取前 50；超过 `truncated=true`

推送 payload 结构 MUST 为：

```json
{
  "type": "notification",
  "seq": <int64>,
  "scope": "direct" | "broadcast",
  "namespace": "<string>",
  "unacked": [<int64>, ...],
  "truncated": <bool>,
  "payload": { ... },
  "created_at": "<RFC3339>"
}
```

`unacked` MUST NOT 落物存储；每次推送时实时派生。

#### Scenario: 稳态推送

- **GIVEN** 用户 U `acked_seq=1000`，收到 seq=1500 的直接消息，无其他未 ack
- **WHEN** 服务端组装 push payload
- **THEN** `unacked = [1500]`, `truncated = false`

#### Scenario: 累积未 ack 场景

- **GIVEN** 用户 U `acked_seq=1000`，累积 seq=1100, 1200, 1500 未 ack
- **WHEN** seq=1500 被 push
- **THEN** `unacked=[1100,1200,1500]`, `truncated=false`

#### Scenario: unacked 截断

- **GIVEN** 用户 U 离线累积 200 条未 ack
- **WHEN** 上线后收到最新 seq 的 push
- **THEN** `unacked` 只包含前 50 个 seq，`truncated=true`

### Requirement: Catchup REST 与首次访问

系统 SHALL 暴露 `GET /api/v1/inbox/messages?since_seq=X&limit=N` 用于客户端拉取。

服务端行为 MUST 为：

1. `state = GetInboxState(userID)`；无行则懒初始化到 `fresh_seq(now)`
2. `effective_since = MAX(since_seq_client, state.acked_seq)`
3. 拉 direct_messages: `WHERE user_id=$u AND seq>$effective_since AND created_at > now()-'30d' ORDER BY seq LIMIT 2*N`
4. 拉 broadcasts: `WHERE seq>$effective_since AND created_at > now()-'30d' ORDER BY seq LIMIT 2*N`
5. 广播结果应用层过滤 targeting
6. 合并按 seq 排序取前 N；若合并后不足 N 但候选未耗尽，`has_more=true`

响应结构 MUST 为：

```json
{
  "messages": [
    {"seq": int64, "scope": "direct"|"broadcast", "namespace": string,
     "payload": {...}, "created_at": "RFC3339"}
  ],
  "acked_seq": int64,
  "has_more": bool
}
```

`limit` MUST 在 `[1, 100]`，默认 50；超出返回 `400 limit_out_of_range`。

#### Scenario: since 被夹紧到 acked_seq

- **GIVEN** 用户 U `acked_seq=5000`
- **WHEN** 客户端传 `?since_seq=100`
- **THEN** 服务端 MUST 按 `effective_since=5000` 起返回

#### Scenario: 首次访问懒初始化

- **GIVEN** 用户 U 无 `user_inbox_state` 行
- **WHEN** U 调用 catchup
- **THEN** 服务端 MUST 分配 fresh_seq、落 `user_inbox_state` 行、返回 `acked_seq = fresh_seq`, `messages=[]`

#### Scenario: 合并广播与单播按 seq 排序

- **GIVEN** direct_messages: seq=100, 500；broadcasts: seq=200, 300（均 targeting 命中）
- **WHEN** catchup limit=10
- **THEN** `messages` 数组顺序 MUST 为 seq: [100, 200, 300, 500]

### Requirement: 每用户全局单一 WebSocket 连接（新连接踢旧连接）

系统 SHALL 保证同一 `user_id` 全局至多存在一条活跃 WS 连接（跨所有客户端类型）。

新连接注册时：
1. 本地 Hub 检查 `map[user_id]` 是否已存在旧连接；存在则调 `SendKicked(new_client_type) + Close(4001)`
2. Hub 通过 Redis Pub/Sub `sub2api:inbox:kick` 广播 `{user_id, new_conn_id, new_client_type}`
3. 其他实例订阅收到，若持有该 user 的连接且 `conn_id != new_conn_id`，同样踢
4. 新连接进入 `map[user_id]`

踢出协议 message MUST 为：

```json
{"type":"kicked", "reason":"opened_elsewhere", "client_type":"<web|ios|android|desktop|unknown>"}
```

紧接着 WS close code MUST = `4001`。

客户端 MUST 区分 close code：
- `4001` → 展示"您已在其他端打开"提示，**不自动重连**
- 其他 code（含 1006 网络异常）→ 指数退避重连

`client_type` MUST 通过 WS 握手 Subprotocol 传递（例如 `Sec-WebSocket-Protocol: inbox.v1, bearer.<token>, ct.web`）；缺失或不在白名单归一为 `unknown`。

v1 registry key MUST 是 `user_id`（不区分 client_type）；`client_type` 只作元数据用于友好提示。

#### Scenario: 同一用户两个 Web tab

- **GIVEN** 用户 U 在 Tab A 已建立 WS
- **WHEN** U 在 Tab B 打开新 WS
- **THEN** Tab A MUST 收到 `{type:"kicked", client_type:"web"}` + close 4001；Tab A MUST NOT 自动重连；Tab B 正常持续

#### Scenario: 跨实例踢出

- **GIVEN** Tab A 连接在实例 1，Tab B 新连接建立在实例 2
- **WHEN** 实例 2 注册 Tab B
- **THEN** 实例 2 通过 Redis Pub/Sub 广播 kick；实例 1 收到后踢 Tab A

#### Scenario: 网络异常不误判

- **GIVEN** WS 因网络异常断开（close code 1006）
- **THEN** 客户端 MUST 走指数退避重连，MUST NOT 显示 kicked 遮罩

### Requirement: WebSocket 鉴权走 Subprotocol

系统 SHALL 通过 WS 握手 `Sec-WebSocket-Protocol` header 传递鉴权 token，MUST NOT 使用 URL query string。

客户端握手 MUST 声明：

```
Sec-WebSocket-Protocol: inbox.v1, bearer.<access_token>, ct.<client_type>
```

服务端 MUST：
1. 解析 `bearer.<token>` → 验证 → 得到 user_id；失败返回 `401` 拒绝握手
2. 解析 `ct.<client_type>`，白名单 `{web, ios, android, desktop}`，其他归一为 `unknown`
3. accept 时回选 `inbox.v1` 子协议

#### Scenario: 无 token 拒绝

- **WHEN** WS 握手不含 `bearer.*` subprotocol
- **THEN** 服务端 MUST 返回 `401`

#### Scenario: token 无效拒绝

- **WHEN** WS 握手 `bearer.` 后跟无效 token
- **THEN** 服务端 MUST 返回 `401`

#### Scenario: 未知 client_type 归一

- **WHEN** WS 握手 subprotocol 含 `ct.foobar`
- **THEN** 服务端接受连接，`client_type` 归一为 `unknown`

### Requirement: 客户端幂等消费（seen_seqs 持久化）

客户端 SHALL 持久化两个键至 `localStorage`：

- `inbox:{user_id}:ack_seq` — 已 ack 水位（number）
- `inbox:{user_id}:seen_seqs` — 已 render 但未 ack 的 seq 列表（JSON array）

**写入时机**：
- `render(seq)` 完成后立即 persist `seen_seqs`
- Server ack 返回 `200` 后 persist `ack_seq` 并从 `seen_seqs` 中裁剪 `≤ ack_seq` 的项

**处理推送 push(msg)** 稳态算法：
1. 若 `msg.seq ∈ seen_seqs`：跳过 render（重复推送幂等），继续第 3 步
2. 否则：`render(msg)`, `seen_seqs.add(msg.seq)`, persist
3. `missing = { s ∈ msg.unacked : s ∉ seen_seqs ∧ s < msg.seq }`；若 `missing 非空` 或 `msg.truncated = true`：调 `catchup(since=local_ack_seq)`
4. 触发 `trySchedulAck`（见下方 Ack 防抖）

**启动流程**：
1. `bootstrapping = true`
2. 从 localStorage 加载 `local_ack_seq`, `seen_seqs`
3. 建立 WS
4. 主动 `catchup(since=local_ack_seq)`
5. Catchup 完成后：drain buffered pushes；`bootstrapping = false`

**登出流程**：清除 `inbox:{user_id}:*` 所有 localStorage 键。

#### Scenario: 已 render 未 ack 时刷新页面不重复消费

- **GIVEN** 客户端已 render seq=1500 未 ack；`local_ack_seq=1400, seen_seqs=[1450, 1500]`，已持久化
- **WHEN** 用户刷新页面，客户端从 localStorage 恢复，catchup 返回 [1450, 1500, 1600]
- **THEN** seq=1450, 1500 MUST 跳过 render（seen_seqs 命中），seq=1600 触发 render；随后 `ack(1600)`（假设 seq 1450, 1500, 1600 连续）

#### Scenario: 收到乱序 seq 触发 catchup

- **GIVEN** 客户端 `local_ack_seq=1000, seen_seqs=[]`
- **WHEN** 收到 push `seq=1500, unacked=[1300, 1400, 1500]`
- **THEN** render 1500；`missing={1300, 1400}` 非空；触发 `catchup(since=1000)`；catchup 返回 [1300, 1400] → 依次 render → `seen_seqs=[1300, 1400, 1500]` → `ack(1500)`

#### Scenario: 累积 ack 只推进连续段末端

- **GIVEN** 客户端 `local_ack_seq=1000, seen_seqs=[1100, 1300]`（1200 缺失）
- **WHEN** 尝试推进 ack
- **THEN** 只能 `ack(1100)`（连续段末端）；`ack(1300)` MUST NOT 被调用

#### Scenario: 登出清理

- **GIVEN** localStorage 存在 `inbox:100:ack_seq`, `inbox:100:seen_seqs`
- **WHEN** 用户执行登出
- **THEN** 两个键 MUST 被移除

### Requirement: 冷启动 bootstrapping 状态

客户端 SHALL 在启动流程期间维护 `bootstrapping = true` 状态直到首次 catchup 完成。

`bootstrapping` 期间处理 push 的行为 MUST 为：

- 只将 push message 入队 `buffered_pushes`
- MUST NOT 调用 render
- MUST NOT 响应 `truncated=true` 触发 catchup
- MUST NOT 触发 gap 探测

首次 catchup 完成后：
- 将 `buffered_pushes` 依次走稳态 `handlePush` 处理（复用同一路径；`seen_seqs` 幂等保证不重复 render）
- 设 `bootstrapping = false`
- 后续 push 走稳态逻辑

#### Scenario: 冷启动只有一次网络往返

- **GIVEN** 客户端刷新页面，同时 WS 建立后服务端立即 push 一条 `truncated=true` 消息
- **WHEN** 客户端启动流程执行
- **THEN** 只发起一次 catchup 请求（冷启动主动那次）；push 中的 `truncated=true` MUST NOT 触发第二次 catchup

#### Scenario: 冷启动期间 push 不丢

- **GIVEN** 冷启动 catchup 进行中，服务端 push 新消息 seq=X
- **WHEN** catchup 完成
- **THEN** 服务端首次 catchup 完成后，客户端 drain 队列，seq=X MUST 被 render（除非 catchup 响应已含 seq=X，此时 seen_seqs 命中跳过 render）

### Requirement: 客户端 Ack 防抖

客户端 SHALL 使用 300ms defer 定时器聚合 ack RPC：

```
pendingAck: int64 | null
ackTimer: Timer | null

trySchedulAck():
  n = 从 local_ack_seq+1 起找 seen_seqs 中最长连续段末端
  if n <= local_ack_seq: return
  pendingAck = n
  if ackTimer == null:
    ackTimer = setTimeout(300, flushAck)

flushAck():
  n = pendingAck
  pendingAck = null; ackTimer = null
  await server.ack(n)
  local_ack_seq = n
  persist(local_ack_seq, 裁剪 seen_seqs)
```

强制 flush 时机：
- `visibilitychange → hidden`
- `beforeunload` → 用 `navigator.sendBeacon` flush（避免异步 ack 丢失）
- WS 断线前

#### Scenario: 密集消息聚合成一次 ack

- **GIVEN** 客户端在 200ms 内连续收到 100 条 push（seq=1..100 连续）
- **WHEN** 300ms defer 超时后
- **THEN** 客户端 MUST 只发送 1 次 `POST /inbox/ack {seq: 100}`，MUST NOT 发送 100 次

#### Scenario: 页面隐藏前强制 flush

- **GIVEN** 客户端 pendingAck=1500, ackTimer 未触发
- **WHEN** `visibilitychange` 事件 hidden 触发
- **THEN** 客户端 MUST 立即执行 flushAck，同步发送 ack RPC

### Requirement: Publisher SDK 与 Namespace 白名单

系统 SHALL 提供内部 Go 接口 `inbox.Publisher`：

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

`namespace` MUST 通过 `inbox.RegisterNamespace(name, metadata)` 白名单注册；未注册 namespace publish MUST 返回 `unknown_namespace`。

`dedupKey` MUST 匹配正则 `^[a-zA-Z0-9:._-]{1,128}$`。

`payload` MUST 是合法 JSON object（非 null、非 array），字节数 ≤ 8192。

Publish 失败 MUST NOT 抛错到调用方主流程；业务方 SHOULD `log.Warn` 后继续。

Feature flag `inbox_v1_enabled=false` 时 publish MUST 静默返回 `(0, nil)`。

#### Scenario: 未注册 namespace 拒绝

- **WHEN** 业务方调 `PublishToUser(ctx, "unknown_ns", "k", 1, ...)`
- **THEN** 返回 `unknown_namespace` 错误，不写入行

#### Scenario: dedupKey 非法拒绝

- **WHEN** publish `dedupKey = "有空格 或 CJK 中文"`
- **THEN** 返回 `invalid_dedup_key` 错误

#### Scenario: Feature flag 关闭时 no-op

- **GIVEN** `inbox_v1_enabled=false`
- **WHEN** 业务方调 `PublishToUser(...)`
- **THEN** 返回 `(0, nil)`；不写入任何行

### Requirement: Targeting 白名单表达式

`Targeting.Filter` MUST 是 JSON 表达式，服务端 MUST 只支持以下 op：

| op | 语法 |
|----|------|
| equals | `{"key": "value"}` 或 `{"key": {"equals": "value"}}` |
| in | `{"key": {"in": [v1, v2, ...]}}` |
| all_users | `{"__all__": true}` |
| and | `{"and": [<expr>, <expr>, ...]}` |
| or | `{"or": [<expr>, <expr>, ...]}` |

其他 op（`regex`, `sql`, `like`, ...）MUST 返回 `400 unsupported_targeting_op`。

Targeting 求值 MUST 通过注册的 `AttributeProvider` 获取用户属性 map，然后递归匹配。系统 MUST 提供默认 `UserBasicProvider`，暴露 `role, country, plan_tier, created_at, id`。

**AttributeProvider 接口 MUST NOT 包含 `QueryUsers` 方法** — fan-out on read 模型不需要在发布时反查用户列表。

#### Scenario: 简单 equals

- **GIVEN** targeting=`{"role":"admin"}`, user attrs=`{"role":"admin","country":"CN"}`
- **THEN** `Match` MUST 返回 true

#### Scenario: and 组合

- **GIVEN** targeting=`{"and":[{"role":"admin"},{"country":{"in":["CN","JP"]}}]}`, attrs=`{"role":"admin","country":"US"}`
- **THEN** `Match` MUST 返回 false

#### Scenario: 全体用户

- **GIVEN** targeting=`{"__all__":true}`
- **THEN** 对任何 attrs MUST 返回 true

#### Scenario: 未支持 op 拒绝

- **WHEN** publish 时 targeting=`{"role":{"regex":"admin.*"}}`
- **THEN** 返回 `400 unsupported_targeting_op`，不写入行

### Requirement: REST 端点

系统 SHALL 暴露以下用户端 REST 端点，全部走 `RequireAuth`：

- `GET /api/v1/inbox/messages?since_seq=X&limit=N` — Catchup
- `POST /api/v1/inbox/ack {seq: int64}` — 累积 ack
- `GET /api/v1/inbox/unacked-count` — 返回 `{count: int64}`
- `GET /api/v1/inbox/ws` — WebSocket 升级端点

系统 SHALL 暴露以下管理端 REST 端点，全部走 `RequireAuth + RequireAdmin`：

- `POST /api/admin/inbox/broadcast {namespace, dedup_key, targeting, payload}`
- `GET /api/admin/inbox/broadcasts?namespace=&page=&page_size=`
- `GET /api/admin/inbox/direct-messages?namespace=&user_id=&page=`

参数校验：
- `limit ∈ [1, 100]`，默认 50，超出返回 `400 limit_out_of_range`
- `since_seq >= 0`
- ack `seq >= 1`

#### Scenario: 未登录 401

- **WHEN** 无 token 调 `GET /api/v1/inbox/messages`
- **THEN** MUST 返回 `401`

#### Scenario: 非管理员访问 admin 端点 403

- **GIVEN** 用户 U 非 admin
- **WHEN** U 调 `POST /api/admin/inbox/broadcast`
- **THEN** MUST 返回 `403`

#### Scenario: limit 越界拒绝

- **WHEN** 调 `GET /api/v1/inbox/messages?limit=1000`
- **THEN** MUST 返回 `400 limit_out_of_range`（不静默截断到 100）

### Requirement: 数据保留与清理

系统 SHALL 通过定时 job 每 24 小时清理旧数据：

- `DELETE FROM direct_messages WHERE created_at < now() - INTERVAL '30 days'`
- `DELETE FROM broadcasts WHERE created_at < now() - INTERVAL '30 days'`
- 单次批量上限 10000 行，分批循环

清理 job MUST 通过 Redis key `sub2api:inbox:cleanup:lock`（SETNX + TTL 300s）抢主，避免多实例并发；未抢到 lock 的实例 MUST no-op。抢到 lock 的实例完成后 MUST 主动 DEL 释放。

清理 MUST NOT 影响 `user_inbox_state.acked_seq`（seq 单调，历史 acked_seq 保留有意义）。

#### Scenario: 保留 30 天

- **GIVEN** 存在 40 天前的 direct_messages 行
- **WHEN** 清理 job 执行
- **THEN** 该行 MUST 被删除

#### Scenario: acked_seq 保留

- **GIVEN** 用户 U `acked_seq=5000`，对应旧消息已被清理
- **WHEN** 清理完成
- **THEN** `user_inbox_state.acked_seq` MUST 仍为 5000

### Requirement: 可观测性与日志脱敏

系统 SHALL 暴露以下 Prometheus 指标：

- `inbox_publish_total{namespace, scope, result}` — counter
- `inbox_seq_alloc_retries_total` — counter（正常应为 0）
- `inbox_ws_connections` — gauge
- `inbox_ws_kicked_total{reason}` — counter
- `inbox_push_total{scope, result}` — counter
- `inbox_catchup_total{result}` — counter
- `inbox_ack_total{result}` — counter
- `inbox_targeting_match_duration_seconds` — histogram

系统 MUST 在关键节点（publish / seq_alloc / ws register/kick / push / catchup / ack）输出结构化日志。

日志 MUST NOT 打印 payload 内容；只允许 `seq`, `scope`, `namespace`, `dedup_key`, `payload_size_bytes`。

#### Scenario: 日志脱敏

- **WHEN** publish payload=`{"secret":"..."}`
- **THEN** 日志 MUST 只含 `seq, scope, namespace, dedup_key, payload_size_bytes`；MUST NOT 出现 `secret` 或其值

#### Scenario: 管理员广播审计

- **WHEN** 管理员 A 调 `POST /api/admin/inbox/broadcast`
- **THEN** MUST 通过现有管理操作审计机制记录 `{admin_id, action:"inbox.broadcast", namespace, dedup_key, seq}`

### Requirement: 灰度开关与回滚

系统 SHALL 通过公开配置 `inbox_v1_enabled` 灰度开关控制通用信箱能力。开关关闭时：

- WS 端点 `GET /api/v1/inbox/ws` MUST 返回 `404`
- REST 端点 `/api/v1/inbox/*` MUST 返回 `404`
- 管理端点 `/api/admin/inbox/*` MUST 返回 `404`
- 业务方调 `PublishToUser` / `PublishBroadcast` MUST 静默 no-op 返回 `(0, nil)`

开关关闭 MUST 不影响：
- 已存在的数据库表和数据
- 后续再次开启后的状态连续性（`acked_seq` 保留）

#### Scenario: 开关关闭 WS 返回 404

- **GIVEN** `inbox_v1_enabled=false`
- **WHEN** 客户端尝试 WS 到 `/api/v1/inbox/ws`
- **THEN** MUST 返回 `404`

#### Scenario: 开关关闭 publish no-op

- **GIVEN** `inbox_v1_enabled=false`
- **WHEN** 业务方调 `PublishToUser(...)`
- **THEN** 返回 `(0, nil)`；不写入任何行；业务主流程正常继续

#### Scenario: 关闭再重开状态保留

- **GIVEN** 关闭前 U `acked_seq=5000`
- **WHEN** 关闭 24h 后重启开启
- **THEN** U `acked_seq=5000` MUST 完整保留
