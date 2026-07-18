## 1. 模块骨架与公共契约

- [x] 1.1 创建 `backend/internal/inbox/` 目录，按 design 建立文件：`service.go` / `publisher.go` / `hub.go` / `ws_handler.go` / `http_handler.go`(REST) / `coordinator.go` / `repository.go` / `targeting.go` / `attribute.go` / `seq.go` / `errors.go` / `metrics.go` / `module.go` / `cleanup.go`（admin broadcast 归并入 http_handler）
- [x] 1.2 定义公共类型：`DirectMessage`, `Broadcast`, `Message`, `PushMessage`, `CatchupResult`, `PublishDirectInput`/`PublishBroadcastInput`, `Targeting`, `Scope('direct'|'broadcast')`, `ClientType('web'|'ios'|'android'|'desktop'|'unknown')`（见 `internal/inbox/types.go`）
- [x] 1.3 定义稳定错误码（`internal/inbox/errors.go`，统一 `INBOX_*` 前缀 + `ApplicationError`）：`INBOX_INVALID_NAMESPACE`, `INBOX_INVALID_DEDUP_KEY`, `INBOX_INVALID_PAYLOAD`, `INBOX_PAYLOAD_TOO_LARGE`, `INBOX_INVALID_TARGETING`, `INBOX_INVALID_RECIPIENT`, `INBOX_INVALID_ACK`, `INBOX_SEQ_ALLOC_EXHAUSTED`, `INBOX_SEQ_ALLOC_FAILED`（ws_kicked / redis_unavailable 待对应 PR 补充）
- [x] 1.4 定义可注入接口：`Repository`, `Publisher`, `Notifier`, `AttributeProvider`, `SeqSource`, `Metrics`, `clientConn`（`Clock` 以注入的 `now func` 形式）
- [x] 1.5 增加 Wire provider 接线（`inbox.ProviderSet` + server 层 6 个 `ProvideInbox*` + `cmd/server` wire.Build）；Coordinator/Cleaner 以后台 goroutine 启动，不阻塞主 API
- [ ] 1.6 实现 `Service.Start(ctx)` / `Shutdown(ctx)` 显式生命周期；禁止构造函数启动 goroutine（当前用 `Coordinator.Run` / `Cleaner.Start` 后台 goroutine + `module.StartBackground`）

## 2. 数据库迁移与 Repository

- [x] 2.1 基于当前最大迁移序号（183）新增 `184_add_general_inbox.sql`，创建三张表：`direct_messages`, `broadcasts`, `user_inbox_state`
- [x] 2.2 `direct_messages` 字段：`seq BIGINT PK`, `user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE`, `namespace VARCHAR(64) NOT NULL`, `dedup_key VARCHAR(128) NOT NULL`, `payload JSONB NOT NULL CHECK (octet_length(payload::text) <= 8192)`, `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`；UNIQUE `(user_id, namespace, dedup_key)`；INDEX `(user_id, seq)` + `(created_at)`
- [x] 2.3 `broadcasts` 字段：`seq BIGINT PK`, `namespace VARCHAR(64) NOT NULL`, `dedup_key VARCHAR(128) NOT NULL`, `targeting JSONB NOT NULL`, `payload JSONB NOT NULL CHECK (octet_length(payload::text) <= 8192)`, `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`；UNIQUE `(namespace, dedup_key)`；INDEX `(created_at)`；INDEX `(seq)` 已通过 PK
- [x] 2.4 `user_inbox_state` 字段：`user_id BIGINT PK REFERENCES users(id) ON DELETE CASCADE`, `acked_seq BIGINT NOT NULL`, `updated_at TIMESTAMPTZ NOT NULL DEFAULT now()`；无默认值（懒初始化）
- [x] 2.5 实现 `Repository.InsertDirectMessage(ctx, seq, in) (created bool, err)`：`INSERT ... ON CONFLICT (user_id, namespace, dedup_key) DO NOTHING`；PK 冲突返回 `ErrSeqConflict`（供上层重试分配）
- [x] 2.6 实现 `Repository.InsertBroadcast(ctx, seq, in) (created bool, err)`：`INSERT ... ON CONFLICT (namespace, dedup_key) DO NOTHING`；PK 冲突返回 `ErrSeqConflict`
- [x] 2.7 实现 `Repository.GetInboxState(ctx, userID) (ackedSeq int64, found bool, err)`：无行返回 `found=false`
- [x] 2.8 实现 `Repository.InitInboxState(ctx, userID, freshSeq) error`：`INSERT ... ON CONFLICT (user_id) DO NOTHING`
- [x] 2.9 实现 `Repository.Ack(ctx, userID, n) error`：`INSERT ... ON CONFLICT DO UPDATE SET acked_seq = GREATEST(existing, EXCLUDED)`（懒 upsert）
- [x] 2.10 实现 `Repository.ListDirectSince(ctx, userID, sinceSeq, limit) ([]Message, error)`：`WHERE user_id=$u AND seq>$s ORDER BY seq LIMIT $n`（30 天保留期在 catchup service 层用 cutoff 参数控制）
- [x] 2.11 实现 `Repository.ListBroadcastsSince(ctx, sinceSeq, cutoff, limit) ([]Broadcast, error)`：`WHERE seq>$s AND created_at>$cutoff ORDER BY seq LIMIT $n`
- [x] 2.12 实现 `Repository.UnackedDirectSeqs(ctx, userID, ackedSeq, currentSeq, limit) ([]int64, error)`（派生 unacked 列表，取代原 Count 方案）
- [x] 2.13 实现 `Repository.UnackedBroadcasts(ctx, ackedSeq, currentSeq, cutoff, limit) ([]Broadcast, error)` — 返回候选（应用层再 targeting 过滤）
- [x] 2.14 清理 job SQL：`Repository.DeleteExpiredDirect` / `DeleteExpiredBroadcasts`（`DELETE ... WHERE seq IN (SELECT seq WHERE created_at < cutoff ORDER BY seq LIMIT N)` 分批）
- [x] 2.15 Repository 集成测试（`routes/inbox_repository_integration_test.go`，PG testcontainer）：单播 dedup 幂等 + seq 主键冲突（ErrSeqConflict）、广播 dedup、ack 单调 + 懒初始化、catchup 分页 + 未 ack 派生、30 天保留删除、审计分页
- [x] 2.16 schema 泄露门禁：`schema_guard_test.go` 解析 `184_add_general_inbox.sql`，断言不含 token/password/secret/authorization/credential/api_key 等敏感字段名

## 3. Redis Seq 分配器

- [x] 3.1 编写 Lua seq 分配脚本（`seq.go` 内嵌 `seqAllocScript`）：使用 Redis `TIME` 拿服务端时钟 + HSET/HINCRBY 记录 last-ms 并 clamp 单调；返回 `ms << 20 | counter`
- [x] 3.2 实现 `SeqAllocator.Next(ctx) (int64, error)` Go 封装：`redis.NewScript(...).Run`（EVALSHA 优先、EVAL 兜底）；Redis 不可用返回错误
- [x] 3.3 脚本预载：`redis.NewScript` 首次 Run 自动 SCRIPT LOAD 并缓存 SHA
- [x] 3.4 实现调用侧 PK 冲突重试封装（`publisher.go`）：循环最多 3 次分配 seq + insert；3 次都冲突返回 `ErrSeqExhausted`
- [x] 3.5 单元测试（`seq_test.go` + miniredis）：
  - 单毫秒并发 1000 次分配，全部单调递增无重复
  - 模拟 Redis TTL 过期后 counter 重置，跨毫秒仍单调
  - 模拟时钟回拨 <2s：新分配仍从上次值+1（因 counter_key 未过期）
  - 模拟时钟回拨 >2s：可能重复分配 → PK 冲突 → 重试成功
  - Redis 不可用返回明确错误
- [x] 3.6 追踪 seq 分配重试：`Metrics.IncSeqAllocRetry()`，在 Publisher 分配-插入循环遇 ErrSeqConflict 时打点；`CountingMetrics.Snapshot().SeqAllocRetries` 可读（`metrics_test.go` 覆盖）

## 4. Publisher SDK 与幂等语义

- [x] 4.1 实现 `PublishToUser(ctx, PublishDirectInput) (created bool, seq int64, err error)`：校验 payload 大小、dedupKey 正则；alloc-insert 重试
- [x] 4.2 实现 `PublishBroadcast(ctx, PublishBroadcastInput) (created bool, seq int64, err error)`：校验 targeting；alloc-insert 重试；成功后经 Notifier 通知 Coordinator 发 Redis pub/sub 广播事件
- [ ] 4.3 实现 `RegisterNamespace(name, metadata)` 白名单注册（当前仅 dedupKey/payload 校验，namespace 由业务侧常量约束，未做集中注册表）
- [x] 4.4 实现 payload 校验：合法 JSON object（非 null、非 array）、`octet_length <= 8192`
- [x] 4.5 实现 dedupKey 正则校验：`^[a-zA-Z0-9:._-]{1,128}$`（`dedupKeyPattern`）
- [x] 4.6 实现 Feature flag 短路：门控上移到业务侧（`SupportTicketNotificationService.inboxReady()` = `inboxEnabled && inboxPub != nil`），关闭时不触达 Publisher
- [x] 4.7 单元测试（`publisher_test.go`）：单播/广播 dedup 幂等、payload 超限、targeting 非法、seq 冲突重试耗尽

## 5. Targeting 与 AttributeProvider

- [x] 5.1 实现 `ParseTargeting` / `ValidateTargeting`：解析 JSON，检查 op 白名单 `{equals, in, all_users, and, or}`，深度限制，拒绝非法结构（`internal/inbox/targeting.go`）
- [x] 5.2 实现 `Targeting.Match(attrs map[string]any) bool`：递归求值（数值跨类型归一化比较）
- [x] 5.3 定义 `AttributeProvider` 接口：`Fetch(ctx, userID) (map[string]any, error)` + `AttributeProviderFunc` 适配器（`attribute.go`）
- [x] 5.4 实现基础 provider：server 层 `ProvideAttributeProvider`（`inbox_glue.go`）读用户返回 `role` 等属性
- [ ] 5.5 实现多 provider 注册与合并（当前单 provider，暂不需要合并层）
- [x] 5.6 实现属性缓存：`NewCachingAttributeProvider(inner, ttl)`（`attribute.go`）
- [x] 5.7 单元测试：`equals` / `in` / `all_users` / `and` / `or` 组合、嵌套表达式、未知 op 拒绝、深度限制、数值跨类型（`internal/inbox/targeting_test.go`）

## 6. Catchup 与 REST 端点

- [x] 6.1 实现 `Service.Catchup(ctx, userID, sinceClient) (CatchupResult, error)`
- [x] 6.2 首次访问懒初始化：`freshBaseline` 分配 freshSeq → `InitInboxState`(ON CONFLICT DO NOTHING) → 重读实际水位
- [x] 6.3 强制 `effective_since = MAX(sinceClient, state.acked_seq)`
- [x] 6.4 拉 direct + broadcast 预取（Config.CatchupLimit / BroadcastScan）
- [x] 6.5 broadcast 结果集在应用层用 `matchTargeting(user_attrs)` 过滤
- [x] 6.6 两路合并按 `seq` 排序取前 limit；不足 limit 且有更多候选 `has_more=true`
- [x] 6.7 返回结构 `{messages:[{seq,scope,namespace,payload,created_at}], acked_seq, has_more}`
- [x] 6.8 REST `GET /api/v1/inbox/catchup?since=X`（命名与 design 的 `/messages?since_seq` 不同，语义一致）
- [x] 6.9 REST `POST /api/v1/inbox/ack {seq}`：调 `Repository.Ack`（GREATEST 语义）；基础范围校验
- [x] 6.10 REST `GET /api/v1/inbox/unread-count`（命名与 design 的 `/unacked-count` 不同，返回 `{count, truncated}`）
- [x] 6.11 REST `POST /api/v1/admin/inbox/broadcast {namespace, dedup_key, targeting, payload}`：走 adminAuth
- [x] 6.12 REST `GET /api/v1/admin/inbox/broadcasts?namespace=&page=&page_size=` 审计查询（分页倒序 + total）
- [x] 6.13 REST `GET /api/v1/admin/inbox/direct-messages?namespace=&user_id=&page=&page_size=` 审计查询
- [x] 6.14 REST handler 专项单元测试（`http_handler_test.go`）：鉴权 401、参数校验 400、Catchup/Ack/UnreadCount/Broadcast 正常路径、审计端点过滤分页

## 7. WebSocket Hub 与推送

- [x] 7.1 实现 `Hub.Register(userID, conn)`：加锁写入连接表；旧连接存在则 `kick` 旧连接（发送 kicked 帧 + 关闭）
- [x] 7.2 通过 Redis Pub/Sub 跨实例踢连接：`inbox:kick` 频道 + 全局唯一 connID（crypto/rand）；Register 广播 kick，各实例 `Hub.KickRemote` 踢 connID 不同的本地旧连接（`hub_test.go` 覆盖）
- [x] 7.3 实现 `Hub.DeliverDirect` / `DeliverBroadcast`：本地持有则组装并投递（broadcast 带 targeting 过滤）
- [x] 7.4 Redis Pub/Sub `inbox:notify:direct`：`Coordinator` 单播发布后广播 `{user_id, ...}`，订阅端投递本地连接
- [x] 7.5 Redis Pub/Sub `inbox:notify:broadcast`：`Coordinator` 广播发布后广播，订阅端遍历本地连接 targeting 命中则 push
- [x] 7.6 实现 `Service.UnackedSeqs` 派生 unacked 列表（供 push 提示 + `truncated`）
- [x] 7.7 实现 WS 握手鉴权（`inbox_glue.go` `inboxWSAuth`）：从 `?token=` 或子协议 `access_token,<jwt>` 解析 token → 校验 → user_id；非法拒绝
- [x] 7.8 实现每连接 ping/pong 心跳 + 读写 pump（`ws_handler.go`）
- [x] 7.9 Hub 单元测试（`hub_test.go`）：单实例踢连接、投递本地连接、未连接用户忽略
- [x] 7.10 WS 服务端端到端测试（`ws_handler_test.go`，httptest + gorilla client，-race）：无效 token 拒绝升级(401)、push 投递、上行 ack 触发 svc.Ack、第二连接踢出旧连接（kicked 帧）。断线重连/bootstrapping 去重属客户端行为，已由前端 `inbox.spec.ts` 覆盖

## 8. 前端：Inbox Store 与 WS 客户端

- [ ] 8.1 创建 `frontend/src/features/inbox/` 目录（改为放在既有约定目录 `frontend/src/stores/inbox.ts` + `frontend/src/api/inbox.ts`，与项目现有 store 组织一致）
- [x] 8.2 创建 `useInboxStore` (pinia setup style)：state `messages` / `localAckSeq` / `seenSeqs` / `connected` / `bootstrapping` / `kicked(+reason,clientType)`；getters `unreadCount` / `hasUnread`；私有 `pendingAck` / `ackTimer` / `pushBuffer`
- [x] 8.3 实现 localStorage 持久化：`inbox:{uid}:ack_seq` / `inbox:{uid}:seen_seqs`；render 后立即 persist seen_seqs；ack 200 后 persist ack_seq + 裁剪 seen_seqs
- [x] 8.4 实现 WS 客户端封装：鉴权走 `?token=`（`buildInboxWsUrl` http→ws）；指数退避重连（kicked 不重连）；`onmessage → handleFrame`
- [x] 8.5 实现 `bootstrap()` 冷启动：`bootstrapping=true` → load 持久化 → `connect()` + `catchup(since=localAckSeq)` → 回放 `pushBuffer` → `bootstrapping=false`（幂等）
- [x] 8.6 实现 `handleFrame` 稳态算法：bootstrapping 期间 buffer；`renderMessage` 去重（seenSeqs）；`truncated`/`unacked` 触发 catchup 兜底；`tryScheduleAck`
- [x] 8.7 实现 `catchup(since)`：调 REST；每条走 `renderMessage` 复用同一幂等路径；has_more 续拉；服务端水位更高时抬升并丢弃已 ack
- [x] 8.8 实现 ack 300ms 防抖：`longestContiguousEnd(localAckSeq, seenSeqs)` → `pendingAck` → `setTimeout(300, flushAck)`；flush 后 persist + 裁剪
- [x] 8.9 实现强制 flush：`visibilitychange→hidden` 调 `flushNow()`；`beforeunload`/`pagehide` 用 `fetch(keepalive)` 上报 ack（需 Authorization 头，故非 sendBeacon）
- [x] 8.10 实现 `kicked` 处理：收到 `{type:"kicked"}` 展示遮罩（`InboxKickedOverlay.vue`）；不自动重连；"在此继续" → `resume()` 重连
- [x] 8.11 实现 `reset()` 清理：`disconnect()` + 移除生命周期监听 + 清定时器/内存状态；App.vue 登出时调用（localStorage 按 uid 保留以便续水位）
- [x] 8.12 单元测试（`inbox.spec.ts`，13 项）：去重、连续段 ack 推进（seenSeqs=[1,2,4] 只 ack(2)）、300ms 防抖聚合、markAllRead、flushNow、kicked、catchup has_more/水位抬升、持久化、reset、WS 连接投递

## 9. 前端：InboxBell 组件与工单 Tab 迁移

- [x] 9.1 修改 `AnnouncementBell.vue` 保持两 Tab 布局（灰度开关 `inboxEnabled` 决定工单 Tab 数据源）
- [x] 9.2 工单 Tab 数据源：`inboxEnabled` 时用 `inboxStore.messages.filter(m => m.namespace==='support_ticket')` 映射成既有 `TicketNotification` 形状
- [x] 9.3 每条消息渲染复用既有列表 UI（title/excerpt/相对时间 + 未读高亮 `seq > localAckSeq`）
- [x] 9.4 点击消息：inbox 模式调 `inboxStore.markReadUpTo(seq)`（累积 ack），不发旧 `notifications/:id/read`
- [x] 9.5 "全部标为已读"：inbox 模式调 `inboxStore.markAllRead()`
- [x] 9.6 未读徽标数：inbox 模式 `announcementUnreadCount + inboxTicketUnread`；旧模式保持原逻辑
- [ ] 9.7 精简 `useTicketUnreadStore`（保留旧字段以支持灰度关闭时回退，暂不移除 `notifications*`/polling）
- [x] 9.8 inbox 模式工单未读由 WS push 实时驱动（替代 60s 轮询）；打开/切 Tab 时不再主动拉取
- [x] 9.9 铃铛 inbox 数据映射抽为纯函数 `announcementBellInbox.ts`（mapInboxTicketItems / countInboxTicketUnread）并单测（`announcementBellInbox.spec.ts`：namespace 过滤、字段映射、is_read 水位推断、未读计数）

## 10. 工单业务迁移（`ticket-notifications` 修改）

- [x] 10.1 namespace 常量 `SupportTicketInboxNamespace = "support_ticket"`（`support_ticket_inbox_bridge.go`）
- [x] 10.2 工单创建 → `publishInboxToAdmins`（PublishBroadcast，targeting role=admin，dedup `ticket_created:<id>`）
- [x] 10.3 管理员回复 → `publishInboxDirect`（PublishToUser ticket.owner，dedup `admin_replied:<reply_id>`）
- [x] 10.4 用户回复 → `publishInboxToAdmins`（PublishBroadcast，dedup `user_replied:<reply_id>`）
- [x] 10.5 payload 结构 `{namespace, event, ticket_id, title, excerpt, actor_name, portal_url}`（字段命名与 design 的 event_type/link_* 略有差异，语义一致）
- [x] 10.6 保留 `support_ticket_reads` 表与"工单红点"逻辑不变
- [ ] 10.7 停止写入 `support_ticket_notification`（当前为双写：旧表 + inbox，便于灰度回退）
- [x] 10.8 旧端点（含 admin）在灰度开启时经 `inboxMigrated` 包装返回 `410 Gone`（`inbox_migration.go`）
- [x] 10.9 email 通知路径不变；inbox 与邮件为独立通道，inbox 发布失败仅 warn，不阻塞工单主流程
- [x] 10.10 桥接单元测试（`support_ticket_inbox_bridge_test.go`）：三触发点发布正确、非法 recipient 跳过、发布失败 swallow、flag 关闭 no-op（端到端 push 集成待补）

## 11. i18n

- [x] 11.1 新增 zh + en 键 `inbox.kicked.title` / `inbox.kicked.description` / `inbox.kicked.resume`（`i18n/locales/{zh,en}/inbox.ts`，并入 index 合并）；空列表/全部已读复用铃铛既有键
- [x] 11.2 工单事件展示复用既有工单通知 UI（数据源改为 inbox payload）
- [x] 11.3 zh + en 键对称且经 `localesMessageCompile` 编译校验（inbox 键无占位符错误）

## 12. 运维与可观测性

- [~] 12.1 Metrics：`Metrics` 接口 + 进程内 `CountingMetrics`（原子计数器，默认启用）已覆盖
  发布(direct/broadcast) / seq 重试 / catchup(次数+条数) / ack / WS 建连 / 踢出 / push 丢弃，
  并已在 publisher/service/hub/ws 各点位打点，可经 `Snapshot()` 读取。
  **Prometheus 导出器 + `/metrics` 端点未接入**：项目当前无 Prometheus 依赖，且公开
  metrics 端点涉及安全/部署决策；导出器future接入时直接读 Snapshot 即可，无需改业务打点。
  （targeting 耗时 histogram 未做）
- [~] 12.2 结构化日志：coordinator / cleanup / bridge 关键节点已有日志（`logger.LegacyPrintf`）；publish/seq_alloc/ws 更细粒度日志待补
- [x] 12.3 日志脱敏：全模块日志仅输出 seq / namespace / count / err，publisher/coordinator/cleanup/bridge 均不打印 payload 内容
- [x] 12.4 管理端审计：`POST /api/v1/admin/inbox/broadcast` 记录结构化操作审计日志（admin id / namespace / dedup_key / seq / created，脱敏不含 payload）
- [x] 12.5 数据保留 job：`Cleaner` 分批（LIMIT 10000）删除超 30 天消息；接入 `LeaderGuard`（Redis `SET NX EX 300s`，`sub2api:inbox:cleanup:lock`）保证多副本每周期仅一个副本执行，锁 TTL 自动释放；无 Redis 时单副本直接执行（`leader.go` + `leader_test.go`）

## 13. 测试与验收

- [x] 13.1 后端 `go test -tags=unit ./...` 通过（含 `./internal/inbox/...`）
- [x] 13.2 后端 `go test -tags=integration ./internal/server/... ./internal/inbox/...` 通过（含 Repository PG 集成测试，Docker 不可用时自动 Skip）
- [~] 13.3 端到端场景（拆分覆盖，未做单一贯通脚本）：
  - 发布单播 → WS 收到 → ack：服务端 `ws_handler_test.go`（push+ack）+ 前端 `inbox.spec.ts`
  - 发布广播 → 匹配/不匹配用户过滤：`hub_test.go`(DeliverBroadcast) + `service_test.go`(Catchup targeting)
  - 离线 → 上线 catchup（fan-out on read）：`service_test.go` Catchup 合并广播
  - gap → 触发 catchup：前端 `inbox.spec.ts`（truncated/unacked → catchup）
  - kicked：`ws_handler_test.go`（第二连接踢旧）+ 跨实例 `hub_test.go`(KickRemote)
  - 首次访问懒初始化：`service_test.go` TestCatchup_LazyInit + 集成 TestInboxRepo_AckMonotonicAndLazyInit
  - 冷启动 bootstrapping 入队：前端 `inbox.spec.ts`
  - ack 防抖聚合：前端 `inbox.spec.ts`（300ms 合并只 ack 一次）
  - 单一 PG+Redis+WS 全链路贯通脚本未做（各段已分别覆盖）
- [x] 13.4 前端 vitest 覆盖 `useInboxStore` 关键算法（`inbox.spec.ts` 13 项通过）
- [~] 13.5 前端 eslint 对新增/改动文件 0 error；`vue-tsc` 类型检查通过；`make test-frontend` 存在既有失败（与 inbox 无关：admin groups/Kiro、ops 日志、support ticket 图片 mock、既有 locale 占位符）
- [x] 13.6 `python3 tools/check_pnpm_audit_exceptions.py --audit frontend/audit.json --exceptions .github/audit-exceptions.yml` 通过（Audit exceptions validated）
- [x] 13.7 后端 `govulncheck ./...`（0 漏洞）与 `golangci-lint run`（0 issues）通过
- [ ] 13.8 手动验收清单：
  - 工单创建 → admin 端 InboxBell 5s 内红点 + Tab 显示新消息
  - 管理员回复 → 用户端信箱收到、点击跳转工单详情
  - 用户关闭 10 分钟 → 重开 → 未读消息全部拉回（如在 30 天内）
  - 用户 Tab A 收到消息未 ack → 刷新 → 消息只渲染一次
  - 用户 Tab A 打开 → Tab B 打开 → Tab A 显示 kicked 遮罩
  - `POST /api/v1/support/tickets/notifications/*` 返回 410
  - 老用户升级后首次开信箱是空的（不 backfill 历史通知），工单红点仍显示

## 14. 上线与回滚

- [x] 14.1 灰度开关：`config.Inbox.V1Enabled`（默认关闭），经 `dto.PublicSettings.inbox_v1_enabled` 下发给前端；前端 App.vue + 铃铛据此门控
- [ ] 14.2 开启前后对比：工单红点数值一致
- [ ] 14.3 回滚预案：
  - 开关一键关闭 → 前端回退到旧 REST 轮询（`support_ticket_notification`）
  - 数据库表保留，不 drop
- [ ] 14.4 灰度周期：内部账号 → 10% → 50% → 全量；每档观察 48h
- [ ] 14.5 观察指标：
  - `inbox_ws_connections` 稳定
  - `inbox_push_total{result="ok"}` 增长
  - `inbox_seq_alloc_retries_total` 保持 0（若 >0 报警）
  - `inbox_ws_kicked_total{reason="opened_elsewhere"}` 在预期范围
