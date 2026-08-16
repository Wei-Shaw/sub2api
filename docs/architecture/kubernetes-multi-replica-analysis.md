# sub2api Kubernetes 多副本部署源码静态分析

> 分析日期：2026-08-04  
> 分析方法：静态分析当前仓库源码，不以 README 的部署描述作为结论依据。  
> 目标架构：Kubernetes 中运行 `sub2api replicas=3`，后端连接 Redis Sentinel 和 PostgreSQL HA。

---

# 一、执行摘要

## 1. 总体结论

当前 sub2api **不能按下面的目标架构原样、无约束地部署**：

```text
Kubernetes
  └─ sub2api replicas=3
       ├─ Redis Sentinel
       └─ PostgreSQL HA
```

主要原因有两类：

1. **Redis Sentinel 不受应用原生支持**：源码固定使用单节点 `redis.NewClient`，没有 Sentinel master discovery、`MasterName` 或 Sentinel 地址列表。
2. **sub2api 不是纯 API 无状态进程**：同一二进制同时启动 HTTP API、缓存订阅器、定时调度器、异步队列 worker、备份 cron 等；其中部分任务缺少跨 Pod claim、租约或 leader lock，多副本运行会重复执行。

更准确的评估如下：

| 范围 | 多副本能力 |
|---|---|
| 核心 HTTP 网关、普通 CRUD | 基本具备多副本基础 |
| 跨 Pod 并发、RPM、Sticky Session | Redis 正常时基本支持 |
| 请求计费和余额扣减 | PostgreSQL 事务及去重设计较好 |
| 可领取型异步 worker | 多数支持多副本 |
| Scheduled Test、Channel Monitor、Backup/Restore | 当前必须单实例 |
| 管理端上游账号 OAuth 流程 | 需要会话亲和或共享 Session Store |
| Redis Sentinel | 不支持直接连接 |
| 完整单体直接设置 `replicas=3` | 不建议 |

## 2. 对五个重点问题的直接回答

### 2.1 sub2api 是否真正无状态？

**否。**

虽然绝大部分持久业务状态位于 PostgreSQL 和 Redis，但进程内仍保存：

- 管理端上游账号 OAuth state 和 PKCE verifier；
- WebSocket 连接及 turn/session 映射；
- Digest Session；
- 本地 L1 cache 和运行时配置快照；
- Usage、Email、Audit 等内存队列；
- 多种 timer、cron、worker pool；
- Pricing、本地页面、日志、setup lock 等文件状态。

### 2.2 哪些模块可以多副本运行？

可以或基本可以多副本运行的模块包括：

- 普通 HTTP API 和 CRUD；
- 普通 Gateway 请求及 SSE；
- PostgreSQL usage billing；
- Redis-backed Account/User/API Key 并发控制；
- Prompt Audit worker；
- Auth cache invalidation outbox worker；
- Batch Image worker；
- API Key Pub/Sub subscriber；
- 有可靠 leader lock 的部分 Ops 聚合和清理任务。

### 2.3 哪些模块当前必须单实例？

- Scheduled Test Runner；
- Channel Monitor Runner；
- Backup Scheduler；
- Backup/Restore 操作；
- 未改造共享 Session Store 前的管理端 OAuth 流程，需要单实例管理面或路由亲和。

### 2.4 是否需要拆分 Deployment？

**需要。**建议拆分为：

1. API Deployment，`replicas=3`；
2. Durable Worker Deployment，`replicas>=2`；
3. Singleton Scheduler/Maintenance Deployment，`replicas=1`；
4. Migration、Backup、Restore 使用独立 Kubernetes Job/CronJob。

但当前源码没有完整的 `api/worker/scheduler` process role。仅修改 Kubernetes YAML 不能阻止每个 Pod 启动同一套后台任务，需要增加进程角色或任务级开关。

### 2.5 目标 Redis Sentinel 是否可用？

**不能直接使用。**不能将应用的 `REDIS_HOST:REDIS_PORT` 直接指向 Sentinel 的 26379 端口。应用不会查询当前 master。

可选方式：

- 修改源码使用 `redis.NewFailoverClient`；或
- 在应用和 Sentinel 之间使用稳定 Primary Endpoint、VIP 或 Redis-aware proxy。

---

# 二、进程启动与运行形态

## 1. 单体进程同时启动 API 和后台任务

应用使用 Wire 将 API、Repository、缓存、worker 和 scheduler 装配成同一个进程：

- 应用初始化入口：`backend/cmd/server/main.go:134-155`
- HTTP Server 启动和关闭：`backend/cmd/server/main.go:166-189`
- 所有核心服务统一构造：`backend/cmd/server/wire_gen.go:35-336`
- 大量 Provider 在构造时直接调用 `Start()`：`backend/internal/service/wire.go:24-565`

统一装配的后台模块包括：

- Ops metrics、aggregation、alert、cleanup、scheduled report；
- Auth cache invalidation worker；
- Scheduler snapshot/outbox worker；
- Token refresh；
- Account/Proxy/Subscription expiry；
- Usage cleanup；
- Idempotency cleanup；
- Batch Image worker/cleanup；
- Scheduled Test Runner；
- Channel Monitor Runner；
- Backup Scheduler；
- Quota Flusher；
- Prompt Audit worker。

这意味着把 Deployment 从 1 扩到 3 时，不只是增加两个 HTTP API 实例，也会增加两套完整后台执行器。

## 2. 当前没有进程角色拆分

当前 `run_mode` 仅有：

- `standard`
- `simple`

见 `backend/internal/config/config.go:22-23,1639-1647`。

它们是业务模式，不是 `api/worker/scheduler` 角色。建议增加：

```text
PROCESS_ROLE=api|worker|scheduler|all
```

以及任务级开关：

```text
SCHEDULED_TEST_RUNNER_ENABLED
CHANNEL_MONITOR_RUNNER_ENABLED
BACKUP_SCHEDULER_ENABLED
PROMPT_AUDIT_WORKER_ENABLED
BATCH_IMAGE_WORKER_ENABLED
MIGRATIONS_ENABLED
```

## 3. 优雅关闭

主进程收到 `SIGTERM` 后先调用 HTTP `Shutdown`，随后 defer 执行完整 Cleanup：

- `backend/cmd/server/main.go:175-189`
- `backend/cmd/server/wire_gen.go:358-678`

后台服务先并行停止，然后顺序关闭 Redis 和 Ent/PostgreSQL：

- Redis Close：`backend/cmd/server/wire_gen.go:625-637`
- 基础设施最后关闭：`backend/cmd/server/wire_gen.go:668-669`

该设计适合 Kubernetes 优雅下线，但 HTTP Shutdown 超时只有 5 秒：`backend/cmd/server/main.go:182-187`。对于长 SSE、WebSocket 和上游长请求通常不足，应结合实际最大请求时长增加 `terminationGracePeriodSeconds` 和应用 drain 时间。

---

# 三、PostgreSQL 状态与多副本正确性

## 1. PostgreSQL 保存的状态

PostgreSQL 是绝大多数持久业务状态的权威源，包括：

- 用户、角色、余额、冻结余额、TOTP 密文；
- API Key、配额、窗口用量；
- Group、账户池、账户凭据、账户状态；
- Proxy 和代理回退状态；
- 订阅、订阅用量、用户平台配额；
- Usage Log、计费去重记录；
- Payment Order、退款及支付审计；
- Redeem Code、Promo Code；
- 系统 Settings、Security Secrets；
- Scheduler Outbox；
- Prompt Audit Job/Event；
- Batch Image Job/Item/Event；
- Scheduled Test Plan/Result；
- Channel Monitor 及历史数据；
- Usage Cleanup Task；
- 通用幂等记录。

主要 schema 位于：

- `backend/ent/schema/user.go`
- `backend/ent/schema/api_key.go`
- `backend/ent/schema/account.go`
- `backend/ent/schema/group.go`
- `backend/ent/schema/user_subscription.go`
- `backend/ent/schema/usage_log.go`
- `backend/ent/schema/payment_order.go`
- `backend/ent/schema/batch_image_job.go`
- `backend/ent/schema/usage_cleanup_task.go`

## 2. 数据库迁移支持多副本启动

每个 Pod 启动都会执行 SQL migration：

- `backend/internal/repository/ent.go:67-75`

迁移使用固定 PostgreSQL session advisory lock：

- 锁定义：`backend/internal/repository/migrations_runner.go:48-52`
- 独占 `*sql.Conn` 获取并持有锁：`backend/internal/repository/migrations_runner.go:124-146`
- 已应用迁移按 filename/checksum 检查：`backend/internal/repository/migrations_runner.go:167-210`

普通迁移把 DDL 和 `schema_migrations` 记录置于同一事务；`*_notx.sql` 只允许受控的并发索引操作。因此多个 Pod 正常并发启动时不会同时执行同一迁移。

尽管源码支持这种模式，生产 Kubernetes 仍建议使用独立 Migration Job，避免 API Pod：

- 启动时竞争迁移锁；
- 在长迁移期间全部 NotReady；
- 新旧版本副本同时运行时发生 schema 兼容问题。

## 3. Usage Billing 的多副本设计较可靠

计费通过 `(request_id, api_key_id)` 进行数据库去重：

- 事务入口：`backend/internal/repository/usage_billing_repo.go:22-63`
- `INSERT ... ON CONFLICT DO NOTHING RETURNING` claim：`backend/internal/repository/usage_billing_repo.go:69-109`
- 订阅、余额、API Key、Account quota 在同一事务应用：`backend/internal/repository/usage_billing_repo.go:174-212`
- 原子余额更新：`backend/internal/repository/usage_billing_repo.go:243-273`

同一请求在不同 Pod 被重复提交计费时，通常只有一个事务会真正应用。

Batch Image 的余额冻结、捕获、释放也复用同类 dedup，并以带余额条件的单条 `UPDATE` 执行：

- `backend/internal/repository/usage_billing_repo.go:124-171`
- `backend/internal/repository/usage_billing_repo.go:275-368`

### 风险：Usage Log 与扣费不是同一事务

Usage Log 自身有 `(request_id, api_key_id)` 唯一索引和 `ON CONFLICT DO NOTHING`，但 Usage Log insert 与 billing transaction 不在同一个事务。

尤其 Batch settlement 可能出现：

```text
capture 扣费成功
  → 标记 settled
  → best-effort 写 Usage Log
```

进程在最后一步前崩溃会产生“已扣费但日志缺失或延迟”的对账窗口。

证据：

- `backend/internal/repository/usage_log_repo_insert.go:212-305`
- `backend/internal/service/batch_image_settlement.go:152-182,252-284`

## 4. 支持多副本的 PostgreSQL Job Claim

### Auth cache invalidation outbox

使用 `FOR UPDATE SKIP LOCKED` claim，并设置 `claimed_by`：

- `backend/internal/repository/auth_cache_invalidation_outbox_repo.go:22-67`

后续 second pass、delete、retry 都校验 `claimed_by`：

- `backend/internal/repository/auth_cache_invalidation_outbox_repo.go:69-130`

属于较可靠的 owner-fenced 多副本消费者。

### Prompt Audit

Prompt Audit 使用：

- `FOR UPDATE SKIP LOCKED`；
- `claim_version`；
- 完成、重试、失败时校验 claim version；
- stale reclaim。

证据：`backend/internal/securityaudit/prompt_repository.go:143-247`。

这是当前最适合多副本 worker 的模块之一。

### Usage Cleanup 的边缘风险

Claim 使用 `SKIP LOCKED` 并可重新领取超时 running task：

- `backend/internal/repository/usage_cleanup_repo.go:119-192`

但 `MarkTaskSucceeded/Failed` 仅按 task ID 更新，没有 owner token 或 claim version：

- `backend/internal/repository/usage_cleanup_repo.go:252-283`

旧 worker 超时后若任务已被新 worker重新领取，旧 worker 仍可能写入终态。建议为任务增加：

- `claimed_by`；
- `claim_version`；
- `lease_expires_at`；
- 完成时 `WHERE id=? AND claimed_by=? AND claim_version=?`。

## 5. Batch Job 幂等键风险

`batch_image_jobs.batch_id` 和 `manifest_hash` 有唯一约束，但 `idempotency_key` 只有普通 partial index。如果入口依赖“先查 idempotency key，再创建”，两个 Pod 并发重试可能创建多个 Job。

账务 hold/capture/release 的 dedup 不能替代 Job 创建唯一性。建议根据实际业务 scope 增加类似：

```sql
UNIQUE (user_id, idempotency_key)
WHERE idempotency_key IS NOT NULL AND idempotency_key <> ''
```

---

# 四、Redis 状态、Sentinel 与故障语义

## 1. Redis 不是纯缓存

Redis 承担以下职责：

| 类别 | Redis 状态 |
|---|---|
| 鉴权 | API Key L2、Refresh Token、Token Family、Passkey Session、验证码、TOTP challenge |
| 并发 | Account/User/API Key slot、Live lease、WS ingress lease、等待计数 |
| 调度 | Scheduler snapshot、version、epoch、active pointer、outbox watermark |
| 会话 | Sticky Session、Session Limit、部分 Live/WS 状态 |
| 限流 | HTTP rate limit、Account/User/Group RPM |
| 计费热状态 | Balance、Subscription、API Key rate windows、User×Platform quota |
| 队列 | Batch Image ready/delayed/active/inflight |
| 分布式锁 | Leader、OAuth Refresh、Redeem、Scheduler Bucket、UMQ |
| 缓存失效 | API Key、Subscription、TLS Profile、Error Rule Pub/Sub |
| 临时故障状态 | timeout/403/500、temp unschedulable |
| 其他 | OAuth Access Token、Fingerprint、Masked Session、Prompt Audit payload |

Redis 丢失会导致的不只是 cache miss，还可能导致：

- 合法 Refresh Token 全部失效；
- Passkey challenge 失效；
- 活跃并发 lease 丢失；
- Batch Queue 丢任务；
- Prompt Audit payload 丢失；
- 短期配额状态丢失；
- `content_moderation:flagged_hashes` 长期集合丢失。

因此 Redis 需要持久化、备份、HA、容量监控和明确的 eviction 策略。

## 2. Redis Sentinel 不受原生支持

客户端固定使用：

```go
redis.NewClient(buildRedisOptions(cfg))
```

证据：`backend/internal/repository/redis.go:23-28`。

配置仅构造普通 `redis.Options`：

- `Addr`
- `Username`
- `Password`
- `DB`
- timeout
- pool
- TLS

证据：`backend/internal/repository/redis.go:31-53`。

Redis 配置模型没有：

- `SentinelAddrs`
- `MasterName`
- Sentinel ACL
- `redis.FailoverOptions`

见 `backend/internal/config/config.go:1444-1467`。

因此：

- 不能把 `REDIS_HOST:REDIS_PORT` 直接指向 Sentinel 26379；
- 应用不会通过 Sentinel 查询 master；
- Sentinel 切主后应用不能依赖 Sentinel 协议自动发现新主。

### 推荐改造

增加 Sentinel 模式配置，并使用：

```go
redis.NewFailoverClient(&redis.FailoverOptions{
    MasterName:       cfg.Redis.MasterName,
    SentinelAddrs:    cfg.Redis.SentinelAddrs,
    SentinelUsername: cfg.Redis.SentinelUsername,
    SentinelPassword: cfg.Redis.SentinelPassword,
    Username:         cfg.Redis.Username,
    Password:         cfg.Redis.Password,
    DB:               cfg.Redis.DB,
})
```

还应支持：

- CA bundle；
- 客户端证书；
- 独立 SNI；
- Sentinel TLS；
- 启动 Ping；
- failover 集成测试。

### 无法立即改源码时

可以使用：

- 托管 Redis 稳定 Primary Endpoint；
- VIP；
- HAProxy/Envoy/其他 Redis-aware proxy。

应用仍使用普通单节点客户端，但连接稳定代理地址。此时必须验证切换期间：

- 客户端重连；
- 写失败窗口；
- 锁和 lease 的复制丢写；
- Pub/Sub 重订阅；
- Lua script reload；
- Refresh Token 和 Batch Queue 的 RPO。

## 3. Redis Cluster 也不能直接替代 Sentinel

源码没有构造 `redis.ClusterClient`，且存在多个跨 key Lua，key 没有统一 hash tag。例如：

- Live lease 同时操作 Account/User/API Key key；
- Scheduler 激活多个不同前缀 key；
- Billing quota 跨多个 key；
- Batch Image enqueue/reserve 操作多个未 hash-tag key。

在 Redis Cluster 中会面临 `CROSSSLOT`。因此不能用 Cluster 代替 Sentinel，而不先系统性改造 key slot。

## 4. 跨 Pod 并发控制

Account/User/API Key 并发使用 Redis ZSET + Lua，并使用 Redis `TIME`：

- 核心 Lua：`backend/internal/repository/concurrency_cache.go:64-180`
- Account/User slot：`backend/internal/repository/concurrency_cache.go:631-750`
- Live lease：`backend/internal/repository/concurrency_cache.go:795-843`
- WS ingress lease：`backend/internal/repository/concurrency_cache.go:205-246,753-793`

Redis 正常时，三个 Pod 共享同一个全局并发上限。

## 5. Scheduler Snapshot

Scheduler 使用 version、active pointer、epoch、retired marker 和 CAS/fencing：

- `backend/internal/repository/scheduler_cache.go:64-219`
- `backend/internal/repository/scheduler_cache.go:413-555`

Redis snapshot miss 时可回 PostgreSQL：

- `backend/internal/service/scheduler_snapshot_service.go:250-299`

### Bucket rebuild lock 缺陷

获取为 `SETNX`，解锁直接 `DEL`：

- `backend/internal/repository/scheduler_cache.go:658-665`

没有 owner token。若第一个 worker 执行超过 TTL，第二个 worker 获得新锁后，第一个 worker 仍可能删除第二个 worker 的锁。

应改为：

- 随机 owner token；
- Lua compare-and-delete；
- 长操作续租；
- 或使用 PostgreSQL advisory lock。

## 6. Batch Image Queue

Batch Queue 的正向设计包括：

- 原子 enqueue；
- 原子 reserve 到 active；
- heartbeat；
- stale active recovery；
- 随机 owner token；
- compare-release/refresh。

证据：

- `backend/internal/repository/batch_image_queue.go:29-82`
- `backend/internal/repository/batch_image_queue.go:181-288`
- `backend/internal/repository/batch_image_queue.go:290-351`

单 Redis Primary 正常时，Batch worker 适合多副本。

## 7. 不可靠的 Redis 锁

以下锁使用 `SETNX` 获取、无条件 `DEL` 释放：

- OAuth Refresh Lock：`backend/internal/repository/gemini_token_cache.go:41-49`
- Redeem Lock：`backend/internal/repository/redeem_cache.go:54-62`
- Scheduler Bucket Lock：`backend/internal/repository/scheduler_cache.go:658-665`

当旧锁过期、其他 Pod 获得新锁后，旧 owner 可能删除新 owner 的锁。Redis failover 或慢操作会提高触发概率。

对比之下，通用 Leader Lock 使用 owner compare-delete：

- `backend/internal/repository/leader_lock_cache.go:14-42`

## 8. Redis 故障行为

| 功能 | Redis 故障行为 |
|---|---|
| Account/User slot acquire | 通常返回错误，请求失败 |
| 等待计数 | Fail-open |
| Session Limit | Fail-open，可能超限 |
| UMQ 串行锁 | Fail-open |
| 普通 HTTP 限流 | 默认 Fail-open |
| 登录等敏感认证限流 | Fail-close |
| 用户/分组 RPM | 多数 Fail-open |
| Balance/Subscription | 回 PostgreSQL；PG 也失败则 Fail-close |
| Refresh Token/Passkey | Redis-only，认证流程失败 |
| Batch Queue | 报错并等待恢复，不应虚假完成 |

因此 Redis 故障时系统不是统一的“全失败”或“全降级”，而是部分请求失败、部分限制失效、部分认证流程中断。

## 9. 主服务没有 Redis 启动 Ping

普通启动只创建客户端，没有 `Ping`：

- `backend/internal/repository/redis.go:23-29`

Redis Ping 只在 setup 测试中：

- `backend/internal/setup/setup.go:266-296`

应用可能在 Redis 不可用时仍监听 HTTP，直到业务请求触发 Redis 错误。应将 Redis writable-primary 检查加入 readiness。

---

# 五、本地内存状态

## 1. 上游账号 OAuth Session

Claude、OpenAI、Gemini、Antigravity、Grok 的账号绑定流程都使用本地 Session Store 保存 state 和 PKCE verifier。

示例：

- Claude：`backend/internal/pkg/oauth/oauth.go:47-117`
- OpenAI：`backend/internal/pkg/openai/oauth.go:52-124`
- 服务调用：`backend/internal/service/oauth_service.go:44-170`
- OpenAI 服务调用：`backend/internal/service/openai_oauth_service.go:17-186`

故障场景：

```text
管理员发起 OAuth → Pod A 保存 state/verifier
浏览器回调 → Pod B
Pod B 本地 map 中没有该 Session
授权失败
```

解决方案：

1. OAuth Session 存 Redis，并一次性原子消费；或
2. 管理端 OAuth 路由使用 Cookie Affinity；或
3. 独立单副本管理面。

## 2. Refresh Token 轮转非原子

当前轮转：

1. GET old token；
2. 校验过期、用户状态、TokenVersion、会话绑定；
3. DEL old token；
4. 生成新 token pair。

证据：

- `backend/internal/service/auth_service.go:1575-1638`
- `backend/internal/repository/refresh_token_cache.go:52-70`

两个 Pod 同时提交同一 Refresh Token 时，可能都在 DEL 前完成校验。建议使用 Lua 原子 consume，并结合 token family reuse detection。

## 3. WebSocket 本地状态

`response_id → account_id` 会写 Redis，但以下内容仅在本地：

- `response_id → conn_id`
- `session → turn_state`
- `session → conn_id`
- 上游 WebSocket connection pool

证据：

- `backend/internal/service/openai_ws_state_store.go:42-88`
- `backend/internal/service/openai_ws_state_store.go:169-288`

连接建立后天然绑定某个 Pod，通常正常；断线重连到另一个 Pod 时，连接复用和部分 turn continuation 可能退化。

## 4. Digest Session

Digest Session 完全使用本地 `go-cache`，TTL 5 分钟：

- `backend/internal/service/digest_session_store.go:20-69`

同一会话切换 Pod 后不会命中旧关联。若业务要求连续性，应迁移 Redis 或使用会话亲和。

## 5. Channel Cache 跨 Pod 不一致

Channel cache 使用本地 `atomic.Value` 和本地 singleflight，TTL 可达约 10 分钟：

- `backend/internal/service/channel_service.go:91-104`
- `backend/internal/service/channel_service.go:146-200`
- `backend/internal/service/channel_service.go:268-289`

某 Pod 修改渠道后，其他 Pod 可能继续使用旧的：

- 渠道启停状态；
- 模型映射；
- 平台路由；
- 定价或倍率。

这不是纯性能缓存问题，而是短期路由和计费正确性问题。建议增加 PostgreSQL Outbox + Redis invalidation，或者每次请求读取带版本号的共享 snapshot。

## 6. 本地安全限流

`invalidAuthAbuseLimiter` 使用本地 sharded map：

- `backend/internal/service/invalid_auth_abuse_limiter.go:11-276`

三个 Pod 经负载均衡后，同一来源的失败尝试预算可能接近放大 3 倍。应迁移到 Redis 原子计数；至少不能将该本地 limiter 作为集群级防爆破边界。

## 7. 本地非持久队列

### Usage Record Worker Pool

- 本地 pond pool：`backend/internal/service/usage_record_worker_pool.go:91-150`
- queue full 时按策略 sync/drop/sample：`backend/internal/service/usage_record_worker_pool.go:153-194`

默认 `sync` fallback 可避免队列满时静默丢计费。但已入本地队列、尚未执行的任务，在 Pod 异常退出时没有跨 Pod 持久投递保障。

### Email Queue

- 本地 channel，容量 100：`backend/internal/service/email_queue_service.go:27-52`
- queue full 返回错误：`backend/internal/service/email_queue_service.go:102-136`

不是持久队列，Pod 崩溃时未发送任务可能丢失。

### Audit Log Queue

- 本地队列容量 4096：`backend/internal/service/audit_log_service.go:12-46`
- queue full 非阻塞丢弃：`backend/internal/service/audit_log_service.go:70-87`
- 优雅关闭时尽量 drain：`backend/internal/service/audit_log_service.go:130-185`

如果审计日志是强合规要求，不能只依赖本地异步队列。

---

# 六、文件系统状态

## 1. Setup 文件

Setup 使用：

- `config.yaml`
- `.installed`
- `DATA_DIR`

证据：

- 数据目录选择：`backend/internal/setup/setup.go:42-74`
- Setup 判断：`backend/internal/setup/setup.go:160-179`
- 写 `.installed`：`backend/internal/setup/setup.go:348-352`
- 写配置：`backend/internal/setup/setup.go:335-343`

Kubernetes 中不应让三个 Pod 同时 `AUTO_SETUP=true`。推荐：

- 初始化使用独立 Job；
- 应用 Pod 设置 `SKIP_SETUP=true`；
- 配置通过 ConfigMap/Secret 注入；
- 不把 `.installed` 当作集群安装状态。

## 2. Pricing 文件

Pricing Service 维护本地内存 map，并读写：

- `model_pricing.json`
- `model_pricing.sha256`

证据：

- 初始化和更新 worker：`backend/internal/service/pricing_service.go:165-250`
- 直接 `os.WriteFile`：`backend/internal/service/pricing_service.go:392-414`
- 默认目录：`backend/internal/config/config.go:2151-2155`

部署影响：

- 每 Pod 独立目录：可能短期使用不同版本价格；
- 多 Pod 共享同一 RWX 目录：没有跨进程 file lock 或原子发布协议，可能互相截断或读到半写文件。

建议使用每 Pod `emptyDir`、镜像内固定文件，或改为共享版本化数据源。

## 3. Pages 和前端覆盖

- Pages：`backend/internal/handler/page_handler.go:27-30,49-128`
- `data/public` 前端覆盖：`backend/internal/web/embed_on.go:301-352`

不同 Pod 内容不同会导致同一路径按 Pod 返回不同页面。若需要动态发布，应使用只读共享发布物和外部原子切换；如果内容固定，最好打入镜像。

## 4. 文件日志

默认日志路径：

- `/app/data/logs/sub2api.log`
- 或 `$DATA_DIR/logs/sub2api.log`

证据：

- `backend/internal/pkg/logger/options.go:10-104`
- `backend/internal/pkg/logger/logger.go:323-341`

不要让多个 Pod 通过 RWX 同时写一个 lumberjack 日志文件。使用 stdout/stderr 和 Kubernetes 日志采集。

## 5. Gateway Debug Body

Gateway 可把请求体 append 到本地文件：

- `backend/internal/service/gateway_service.go:1376-1387`

可能包含 Prompt 和敏感 metadata。生产中应默认禁用，不应写入共享持久卷。

## 6. 在线更新和回滚

应用内 update 会下载、rename 当前可执行文件和备份：

- `backend/internal/service/update_service.go:215-299`

在 Kubernetes 中：

- 只会修改某一个 Pod；
- 其他 Pod 版本不变；
- Pod 重建后恢复镜像版本；
- 共享可执行文件更危险。

必须用镜像 tag、Deployment rollout、Helm 或 GitOps 更新，不应使用应用内更新/回滚/重启接口。

---

# 七、模块多副本分类

## 1. 可以多副本运行

| 模块 | 判断 | 依据或限制 |
|---|---|---|
| 普通 HTTP API / CRUD | 可以 | 权威状态位于 PostgreSQL |
| Gateway 普通请求 / SSE | 可以 | Redis Lua 管理 Account/User/API Key 并发 |
| PostgreSQL Migration | 技术上可以 | PG advisory lock 串行；仍建议独立 Job |
| Usage Billing | 可以 | PG transaction + request dedup |
| API Key Auth Cache | 可以 | L1 + Redis L2 + Pub/Sub + PG Outbox |
| Scheduler 请求读取 | 可以 | Redis snapshot + DB fallback |
| Prompt Audit worker | 可以 | `SKIP LOCKED + claim_version` |
| Auth invalidation worker | 可以 | `SKIP LOCKED + claimed_by` |
| Batch Image worker | 可以 | Redis reserve、owner lock、heartbeat、recovery |
| Account Expiry | 可重复运行 | 条件 UPDATE，状态转换基本幂等 |
| Audit retention | 可重复运行 | 删除操作幂等 |
| Leader-lock 保护的 Ops 任务 | 可以多副本启动 | Redis/PG lock backend 必须可靠 |
| API Key Pub/Sub subscriber | 每 Pod 都应运行 | 清理每 Pod 的 L1 cache |

## 2. 可以重复运行但会增加成本或存在边缘风险

### Idempotency Cleanup

每 Pod 都运行本地 ticker，无 leader lock，但操作只是删除过期记录：

- `backend/internal/service/idempotency_cleanup_service.go:42-91`
- `backend/internal/repository/idempotency_repo.go:217-237`

通常不会破坏正确性，但会重复扫描数据库。

### Proxy Expiry

无 leader lock，依赖事务和条件状态收敛：

- `backend/internal/service/proxy_expiry_service.go:23-63`
- `backend/internal/repository/proxy_repo.go:631-678`

并发重复通常会收敛，但可能产生重复日志、额外 outbox 或短期快照差异。

### Batch Image Cleanup

每 Pod 本地 ticker，无跨 Pod lock。终态字段可防止部分重复，但多个 Pod 仍可能同时调用外部对象删除。

### Scheduler Snapshot Worker

每 Pod 都应维护可用 snapshot，但会重复轮询和尝试 rebuild。其数据发布有 fencing，bucket lock 仍需修复 owner token。

## 3. 当前必须单实例运行

### Scheduled Test Runner

每个 Pod 都启动每分钟 cron：

- `backend/internal/service/wire.go:492-503`
- `backend/internal/service/scheduled_test_runner_service.go:45-67`

Due plan 查询是普通 SELECT，没有 claim/lease：

- `backend/internal/repository/scheduled_test_repo.go:51-63`

每个副本都会调用上游测试、写 result，并更新 `next_run_at`：

- `backend/internal/service/scheduled_test_runner_service.go:87-146`
- `backend/internal/repository/scheduled_test_repo.go:80-112`

三个副本可能执行三次同一计划。

### Channel Monitor Runner

每个 Pod 启动时加载全部 enabled monitor，并为每个 monitor 建本地 timer：

- `backend/internal/service/channel_monitor_runner.go:115-198`
- `backend/internal/service/channel_monitor_runner.go:236-295`

`inFlight` 只在本进程：

- `backend/internal/service/channel_monitor_runner.go:56-65,278-295`

三个 Pod 会重复探测并重复写历史。

### Backup Scheduler / Backup / Restore

`backingUp`、`restoring` 和 cron 都是本地状态：

- `backend/internal/service/backup_service.go:118-149`
- `backend/internal/service/backup_service.go:171-192`
- `backend/internal/service/backup_service.go:383-473`
- `backend/internal/service/backup_service.go:741-753`

多副本可能并发备份或并发 Restore。Restore 应严格做成人工触发 Job。

### 管理端上游账号 OAuth

不要求整个 API 单副本，但未共享 Session Store 前必须满足以下之一：

- 管理 OAuth 路由 affinity；
- 单副本管理面；
- Redis Session Store。

### 应用内 Update/Rollback/Restart

不适用于 Kubernetes 多副本，必须由集群部署控制面管理。

---

# 八、推荐 Kubernetes 部署方案

## 1. API Deployment

```text
sub2api-api
replicas: 3
```

运行：

- HTTP API；
- Gateway；
- API Key L1 cache；
- Redis Pub/Sub subscriber；
- 请求级并发控制；
- 请求级计费提交。

禁用：

- Scheduled Test；
- Channel Monitor；
- Backup cron；
- Restore；
- 在线更新。

普通 HTTP/SSE 请求一般不需要 sticky session。上游账号 OAuth 路由和部分 WS 重连场景需要 affinity，直到状态共享改造完成。

## 2. Durable Worker Deployment

```text
sub2api-worker
replicas: 2+
```

可运行：

- Prompt Audit；
- Auth cache invalidation outbox；
- Batch Image worker；
- 修复 completion fencing 后的 Usage Cleanup；
- 其他具有 DB claim 或可靠 owner-token lock 的消费者。

## 3. Scheduler/Maintenance Deployment

```text
sub2api-scheduler
replicas: 1
```

放入当前缺少分布式 claim 的任务：

- Scheduled Test；
- Channel Monitor；
- 暂未完成可靠锁改造的周期任务。

长期应为这些任务增加 DB lease/claim，使 scheduler 也可多副本部署。

## 4. 独立 Job/CronJob

- Migration：Deployment 发布前运行的 Job；
- Backup：Kubernetes CronJob；
- Restore：人工触发 Job；
- 一次性 cleanup：Job。

## 5. PostgreSQL HA

sub2api 使用单一 host/port DSN：

- `backend/internal/config/config.go:1385-1440`
- `backend/internal/repository/ent.go:45-65`

应连接稳定 Writer Endpoint，例如：

- Patroni writer Service；
- RDS/Aurora/Cloud SQL writer endpoint；
- HAProxy/PgBouncer writer endpoint。

应用没有读写分离或多主机自动发现。

数据库连接池是每 Pod 独立的：

- `backend/internal/repository/db_pool.go:31-68`

例如每 Pod `max_open_conns=50`，3 个 API Pod 就可能使用 150 个连接；再加 worker 和 scheduler 后更高。必须按所有 Pod 总和预算数据库连接数。

## 6. Redis HA

优先级建议：

1. 修改应用为原生 Sentinel client；
2. 或使用稳定 Primary Proxy/VIP；
3. Redis 启用持久化，并明确 RPO；
4. 避免会淘汰 Refresh Token、Lock、Queue 的 eviction policy；
5. 监控 replication lag、failover、reconnect、Lua error 和 Pub/Sub subscriber 状态。

## 7. Secret 一致性

所有 Pod 必须共享相同 Secret：

- JWT Secret；
- `TOTP_ENCRYPTION_KEY`；
- 数据库凭据；
- Redis 凭据；
- 对象存储凭据；
- 支付和其他持久密文使用的密钥。

JWT Secret 未配置时，代码会使用 PostgreSQL `security_secrets` 进行 get-or-create，帮助保持跨实例一致：

- `backend/internal/repository/security_secret_bootstrap.go:27-57`

但 TOTP Encryption Key 未配置时每个进程会独立生成随机值：

- `backend/internal/config/config.go:1791-1803`

因此多 Pod 必须显式配置固定 `TOTP_ENCRYPTION_KEY`，否则不同 Pod 无法一致解密持久 Secret。

## 8. 文件系统

建议：

- 配置由 ConfigMap/Secret 提供；
- 每 Pod 本地临时数据使用 `emptyDir`；
- 不共享可写 Pricing 文件；
- 不共享 lumberjack 日志文件；
- Pages/static override 固定时打入镜像；
- 动态大对象放 S3/R2/GCS；
- 日志输出 stdout/stderr；
- 禁用 debug body 持久落盘。

---

# 九、健康检查与可观测性

当前 `/health` 固定返回：

```json
{"status":"ok"}
```

证据：`backend/internal/server/routes/common.go:9-14`。

它不检查：

- PostgreSQL；
- Redis；
- Migration；
- Scheduler snapshot；
- 必需 Secret；
- Worker 和 Outbox lag。

建议拆分：

```text
/livez
  只检查进程和 HTTP event loop 是否存活

/readyz
  检查 PostgreSQL writer
  检查 Redis writable primary
  检查 migration 完成
  检查关键 Secret 和初始化
  视角色检查必要 worker/scheduler 状态
```

不要把所有 worker lag 都作为 API Pod readiness 的硬条件，否则后台积压可能导致 API 全部被摘流。应按 process role 定义 readiness。

建议监控：

- PostgreSQL pool 使用率和等待；
- Redis command error、timeout、reconnect；
- Sentinel failover 时间；
- Auth outbox lag；
- Scheduler outbox watermark/lag；
- Prompt Audit queued/retry/processing；
- Batch active/stale/delayed；
- Usage worker drop/sync fallback；
- Audit queue dropped；
- Channel/Scheduled Test 重复执行指标；
- Redis key 数量、内存和 eviction；
- `content_moderation:flagged_hashes` 增长。

---

# 十、上线阻断项和改造优先级

## P0：上线前阻断项

1. 不要将普通 Redis 客户端直接连接 Sentinel 端口；改源码或使用稳定 Primary Proxy。
2. Scheduled Test 只能由一个实例执行。
3. Channel Monitor 只能由一个实例执行。
4. Backup/Restore 与 API Pod 隔离。
5. 管理端上游 OAuth 使用 affinity 或共享 Redis Session。
6. 修复 Refresh Token 原子轮转。
7. 所有 Pod 配置相同固定 `TOTP_ENCRYPTION_KEY`。
8. 禁止多个 Pod 同时 `AUTO_SETUP=true`。
9. 禁用应用内 self-update、rollback、restart。
10. 增加真正的 readiness。

## P1：高优先级正确性改造

11. OAuth Refresh Lock 增加 owner token、compare-delete 和续租。
12. Scheduler Bucket Lock 增加 owner token。
13. Redeem Lock 增加 owner token。
14. Channel cache 增加跨 Pod invalidation。
15. 将 invalid-auth abuse limiter 迁移 Redis。
16. Usage Cleanup completion 增加 claim fencing。
17. Batch Job idempotency key 增加数据库唯一约束。
18. 关键异步任务改用 PostgreSQL Outbox 或持久消息队列。
19. 修复 Usage Log 与 billing 的对账窗口，至少引入可靠补偿事件。

## P2：稳定性和运维改造

20. 增加 process role 和任务级开关。
21. WebSocket 和 SSE 增加 drain 机制及更长终止时间。
22. 定义每项限流究竟是 per-Pod 还是 global。
23. 对 Pricing 和动态设置使用版本化共享数据源。
24. 为 Redis failover 建立集成测试和演练。
25. 按 API/worker/scheduler 角色拆分指标和告警。

---

# 十一、最终总结

## 1. 是否支持 Kubernetes 多副本？

**核心 API 数据面基本具备水平扩展基础，但当前完整单体不支持无约束地直接设置 `replicas=3`。**

## 2. 是否真正无状态？

**不是。**持久业务状态大多外置，但仍存在本地 OAuth Session、本地队列、本地缓存、本地 WS/Digest 会话、本地 cron 和本地文件状态。

## 3. Redis Sentinel 是否支持？

**不支持原生 Sentinel discovery。**必须修改为 `redis.NewFailoverClient`，或通过稳定 Primary Endpoint/Proxy 屏蔽切主。

## 4. 哪些模块必须单实例？

当前至少包括：

- Scheduled Test Runner；
- Channel Monitor Runner；
- Backup Scheduler；
- Backup/Restore；
- 未共享 Session Store 前的管理端 OAuth 流程需要单实例管理面或路由亲和。

## 5. 是否需要拆分 Deployment？

**需要。**推荐：

```text
API Deployment             replicas=3
Durable Worker Deployment  replicas>=2
Scheduler Deployment       replicas=1
Migration Job              one-shot
Backup CronJob             singleton schedule
Restore Job                manual one-shot
```

## 6. 最终架构建议

```text
Ingress / Load Balancer
          |
          +------------------------------+
          |              |               |
      API Pod 1      API Pod 2       API Pod 3
          |              |               |
          +-------- PostgreSQL HA Writer Endpoint
          |
          +-------- Redis Sentinel-aware Client
                    或稳定 Redis Primary Proxy

Durable Workers replicas>=2
  - Prompt Audit
  - Auth Outbox
  - Batch Image
  - 其他具备 claim/fencing 的 worker

Singleton Scheduler replicas=1
  - Scheduled Test
  - Channel Monitor
  - 尚未改造分布式 claim 的维护任务

Kubernetes Jobs/CronJobs
  - Migration
  - Backup
  - Restore
```

在完成 Sentinel 接入、任务拆分、OAuth Session 共享、Refresh Token 原子轮转、关键缓存失效和若干锁修复之前，若 Scheduled Test、Channel Monitor、Backup 等功能已经启用，最保守的部署仍是：

```text
完整 sub2api 单体 replicas=1
```

而不是直接把当前完整单体扩为三个副本。
