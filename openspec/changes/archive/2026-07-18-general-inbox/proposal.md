## Why

当前项目的实时消息通知能力散落在两处，且都不满足"通用、一次且仅一次、可扩展"的要求：

1. `support-ticket` capability 通过 `support_ticket_notification` 表 + 前台 60s 轮询 + `AnnouncementBell` 组件承担工单通知；只覆盖工单事件、依赖轮询、且强绑定 `support_tickets` 外键，无法复用到其他业务。
2. 站内公告（announcements）走另一条路径，读游标语义与工单不同。
3. 邀请返利、余额变动、图像任务完成、系统运维公告等新增业务只能选择"再加一张表 + 一个 store + 一轮轮询"或"塞进工单表"两个都很坏的选项。

业务上真正需要的是一个**通用信箱模块**：

- 任何业务方通过一个"发布"接口就能把一条消息投递给一个用户（单播）或按属性过滤的一批用户（广播）。
- **存储严格一次**（同一业务事件多次触发 publish 只落一条）、**推送至少一次**（WS 断线/重连时自动补齐）、**消费恰好一次**（客户端幂等去重 + 累积 ack 保证不重复消费）。
- 每条消息在全局有一个单调递增的 `seq`，客户端可以据此排序、探测 gap、累积 ack。
- WebSocket 主推 + REST 兜底拉取；同一用户全局只有一条活跃 WS 连接，新连接踢旧连接。
- 广播支持**属性目标（attribute-based targeting）**：例如"role=admin"、"country ∈ [CN,JP]"，由业务方注册属性提供者。

## What Changes

### 新增能力

- 新增 `general-inbox` capability：定义两张独立表（`direct_messages` 单播 + `broadcasts` 广播）、Redis 分配的**全局统一** seq、累积 ack、fan-out on read 的广播、WebSocket 协议、REST catchup 接口、client_type 元数据、发布 SDK 和管理员运维接口。

### 修改能力

- 修改 `ticket-notifications` capability：把"工单通知记录"从 `support_ticket_notification` 表 + 独立轮询，改为通过通用信箱发布 `namespace=support_ticket` 的消息；工单未读红点（基于 `support_ticket_reads`）保留独立语义不变。

### 关键设计决策（本轮拷问最终确定项）

- **两张独立表，不引入中间 delivery 表**：
  - `direct_messages`：每用户每消息一行，`UNIQUE (user_id, namespace, dedup_key)` 保证单播幂等
  - `broadcasts`：全局一份，`UNIQUE (namespace, dedup_key)` 保证广播幂等；不 fan-out
- **广播采用 fan-out on read**：发布只写一行 broadcasts + Redis Pub/Sub 通知在线用户；离线用户在 catchup 时由服务端按 targeting 二次过滤。发布路径 O(1)，不因用户数增加变慢。
- **全局统一 seq 空间**：direct_messages 与 broadcasts 共用同一个 Redis 分配的 `BIGINT seq`（`seq = redis_time_ms << 20 | INCR_within_ms`），客户端只维护一套 `local_ack_seq` + `seen_seqs`。
- **首次访问懒初始化 acked_seq**：`user_inbox_state` 无行时，服务端调用 `fresh_seq(now)` 作为初始水位。新用户和老用户迁移都从此刻开始"看新消息"，不 backfill 历史。
- **累积 ack**：客户端只在**连续无洞**的 seq 段末端推进 ack；服务端 `UPDATE ... SET acked_seq = GREATEST(acked_seq, $n)` 单调抬升，无覆盖问题。
- **推送携带 `unacked` 列表**：从 `direct_messages ∪ broadcasts(targeting matches)` 中派生 `(acked_seq, current_seq]` 前 50 项，超过则 `truncated=true` 触发客户端拉取。
- **客户端持久化 `local_ack_seq` 与 `seen_seqs`**：处理"已 render 未 ack 时刷新页面"的重复消费问题。
- **客户端 ack 防抖**：300ms defer，密集消息聚合成一次 `ack(max_seq)` RPC；页面隐藏 / 关闭前强制 flush。
- **冷启动 `bootstrapping` 状态**：首次 catchup 完成前不响应 push 中的 `truncated=true`，避免重复触发拉取。
- **每用户全局单一 WS**：新连接踢旧连接；踢出协议携带 `client_type`（web/ios/android/desktop/unknown）用于友好提示。v1 registry key 为 `user_id`；v2 若切多端并存只需改 registry key，协议不变。
- **广播过期由业务方在 payload 里自决**：不引入 `starts_at` / `ends_at` / `archived_at`；服务端只做统一 30 天硬删。

### 迁移与兼容

- **不 backfill 历史通知**：老用户升级后首次开信箱是"空的"，历史通知不进入新信箱。用户仍能通过工单列表红点（`support_ticket_reads` 保留）感知未读工单。产品语义定为"信箱系统从上线时刻启用"。
- 旧端点 `/api/v1/support/tickets/notifications*` 保留一版观察期返回 `410 Gone`，下一次 release 删除。
- `support_ticket_notification` 表保留一版观察期，不再写入新记录；v2 release 决定是否 drop。

## Impact

### 后端

- 新增 `backend/internal/inbox/` 模块（`service.go` / `publisher.go` / `hub.go` / `ws_handler.go` / `rest_handler.go` / `admin_handler.go` / `repository.go` / `targeting.go` / `attribute.go` / `seq.go` / `errors.go`）
- 新增数据库表：`direct_messages`、`broadcasts`、`user_inbox_state`
- 新增 REST：
  - `GET /api/v1/inbox/messages`（catchup）
  - `POST /api/v1/inbox/ack`
  - `GET /api/v1/inbox/unacked-count`
  - `GET /api/v1/inbox/ws`（WebSocket 升级）
- 新增管理员 REST：`POST /api/admin/inbox/broadcast`、`GET /api/admin/inbox/broadcasts`、`GET /api/admin/inbox/direct-messages`
- 新增内部 SDK `inbox.Publisher`：`PublishToUser(...)` 和 `PublishBroadcast(...)`
- 新增 `AttributeProvider` 接口 + 默认 `UserBasicProvider`
- 新增 Redis：
  - Seq 分配 Lua script（key `seq:inbox:global`）
  - Pub/Sub 通道 `sub2api:inbox:broadcast` 跨实例广播消息事件
  - Pub/Sub 通道 `sub2api:inbox:kick` 跨实例踢连接

### 前端

- 新增 `frontend/src/features/inbox/`：`useInboxStore`（pinia）、WS 客户端封装、localStorage 持久化 `local_ack_seq` + `seen_seqs`、`bootstrapping` 状态、ack 300ms 防抖
- 修改 `AnnouncementBell.vue`：工单 Tab 数据源改为信箱 store 按 `namespace='support_ticket'` 过滤
- 修改 `useTicketUnreadStore`：仅保留 `unreadCount`（工单详情未读，基于 `support_ticket_reads`），移除 `notifications*` 和轮询
- 移除 60 秒背景轮询定时器；改为路由切换 / `visibilitychange` / WS push 触发刷新

### 数据库

- 新增三张表 + 索引
- 30 天硬删清理 job
- 不 drop、不迁移 `support_ticket_notification`（保留观察期）

### 安全与隐私

- `payload` 硬限 8 KiB
- `dedup_key` 长度 ≤128，正则限制字符集
- Targeting 白名单表达式（`equals`, `in`, `all_users`, `and`, `or`）
- WS 鉴权走 Subprotocol（避免 access log 泄露 token）
- 日志脱敏：不打印 payload 内容

### 性能

- **广播发布 O(1)**：不论 1 万还是 100 万用户，只写一行 + 一次 Pub/Sub。
- **广播 catchup**：查近 30 天 broadcasts 表（预估 <3000 行）+ 应用层 targeting 过滤，成本可控。
- **direct_messages 热表**：主键 `(seq)`，覆盖索引 `(user_id, seq)`；预期日增 = 日活 × 单播消息数（估算 10 万级/天可控）。
- **Redis seq 分配**：Lua 脚本单次 <1ms；不占用 DB 行锁。

### 兼容性

- 无外部 API breaking change
- WS 是新增端点，旧客户端不受影响
- 旧通知端点返回 410 引导迁移

## Non-goals

- **不做消息编辑/撤回**：撤回通过发一条 `dedup_key=recall:<orig_seq>` 的新消息实现，客户端按业务约定处理。
- **不做消息优先级 / 置顶 / 分类目录**：v1 只区分 namespace 一层。
- **不做已读跨端 UI 事件推送**：`acked_seq` 是账号级共享的，其他端上线走 catchup 拿最新水位即可。
- **不做移动端 App 独立连接**：v1 全局单 WS，`client_type` 只作元数据。
- **不做消息归档冷表**：v1 只做热表 + 30 天硬删。
- **不 backfill 存量工单通知历史**：新旧系统语义切换，从上线时刻启用。
- **不做移动推送（APNs / FCM）**：留给未来独立 capability。
- **不做消息搜索**。
