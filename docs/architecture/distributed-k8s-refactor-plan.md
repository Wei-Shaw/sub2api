# sub2api 分布式/Kubernetes 高可用改造计划

## 1. 文档目标

本文档针对当前 sub2api 单体架构，制定一套**最小改造、保留现有业务逻辑和功能**的高可用改造方案。

目标：

1. 解决当前单体服务的 API 单点故障问题。
2. 不修改现有用户可见功能、业务流程、API 协议、计费规则和上游适配逻辑。
3. 使项目可以通过 Kubernetes 部署和滚动升级。
4. 支持 PostgreSQL HA 和 Redis Sentinel/稳定 Redis Primary Endpoint。
5. 不引入新的业务功能，不立即拆成多个微服务。
6. 保留现有单机 Docker Compose 部署能力。

核心原则：

> 不把 sub2api 重写成微服务，而是将当前单体改造成“按运行角色启动的模块化单体”。

也就是说：

- 保留同一个代码库；
- 保留同一个 Go 模块；
- 保留同一个 Docker 镜像；
- 保留现有 Handler、Service、Repository 和业务逻辑；
- 通过启动角色决定一个 Pod 启动哪些既有模块；
- 通过 PostgreSQL、Redis 和现有队列/Outbox 机制进行跨 Pod 协作。

---

## 2. 当前问题概述

当前应用初始化时，会在同一个进程中同时启动：

```text
HTTP API
Gateway / 协议转换
用户和 API Key 鉴权
账号调度
Redis 缓存订阅
Usage 记录 worker
Batch Image worker
Prompt Audit worker
Scheduled Test
Channel Monitor
Backup Cron
Token Refresh
Account/Proxy/Subscription Expiry
Ops Cleanup / Aggregation / Report
各类本地定时器和内存队列
```

主要启动和装配位置：

- `backend/cmd/server/main.go:134-189`
- `backend/cmd/server/wire.go`
- `backend/cmd/server/wire_gen.go:35-336`
- `backend/internal/service/wire.go:24-565`
- `backend/internal/service/wire.go:731-856`

如果只把当前镜像设置为：

```yaml
replicas: 3
```

实际效果是：

```text
3 个 HTTP API
3 个 Prompt Audit worker
3 个 Batch worker
3 个 Scheduled Test cron
3 个 Channel Monitor runner
3 个 Backup cron
3 套本地内存队列
3 套本地缓存
```

其中一部分可以水平扩展，但以下模块会重复执行或存在单机状态问题：

- Scheduled Test Runner；
- Channel Monitor Runner；
- Backup Scheduler；
- Backup/Restore；
- OAuth 管理端临时 Session；
- 本地 Email/Audit/Usage 队列；
- 部分本地缓存和运行时设置。

因此不能只修改 Kubernetes YAML，而必须先增加角色化启动能力。

---

## 3. 目标部署结构

```text
                          Internet
                             │
                    Ingress / Load Balancer
                             │
       ┌─────────────────────┼─────────────────────┐
       │                     │                     │
┌──────▼──────┐       ┌──────▼──────┐       ┌──────▼──────┐
│ Gateway Pod │       │ Gateway Pod │       │ Gateway Pod │
│ role=api    │       │ role=api    │       │ role=api    │
└──────┬──────┘       └──────┬──────┘       └──────┬──────┘
       └─────────────────────┼─────────────────────┘
                             │
                  PostgreSQL HA RW Endpoint
                             │
                    Redis Sentinel / Proxy
                             │
              ┌──────────────┴──────────────┐
              │                             │
      ┌───────▼────────┐          ┌─────────▼─────────┐
      │ Worker Pods    │          │ Scheduler Pods    │
      │ role=worker    │          │ role=scheduler    │
      │ replicas >= 2  │          │ replicas = 2      │
      └────────────────┘          │ leader-only work  │
                                  └───────────────────┘

Kubernetes Jobs:
  - sub2api-migrate
  - sub2api-bootstrap（首次安装才需要）
  - sub2api-restore（人工触发）
```

### 3.1 运行角色

| Role | 第一阶段副本数 | 职责 |
|---|---:|---|
| `all` | 1 | 保持当前单机行为，兼容 Docker Compose |
| `api` | 2～3 | HTTP API、Gateway、鉴权、调度读取、转发、SSE/WS、同步计费 |
| `worker` | 1，验证后 2+ | 可异步执行的后台任务 |
| `scheduler` | 2，但同一时刻只有 1 个 leader | 定时任务和维护类循环 |
| `migrate` | Kubernetes Job | 数据库迁移和启动校验后退出 |
| `bootstrap` | 一次性 Job | 首次安装、数据库初始化、管理员初始化 |
| `restore` | 人工触发 Job | 数据恢复，不与在线服务混跑 |

---

## 4. 改造边界

### 4.1 保留不变的内容

以下内容原则上不改变：

- 现有 HTTP API 路径；
- 现有 OpenAI、Anthropic、Gemini 等协议入口；
- 用户认证和 API Key 认证规则；
- Group、Account、Channel、Proxy 的业务模型；
- 账号调度算法；
- 现有余额、订阅、配额和 Usage 规则；
- 现有上游 Token 获取和协议转换逻辑；
- 现有前端功能；
- 现有 PostgreSQL 业务表语义；
- 现有 Redis Key 的业务语义；
- 现有 Batch Image、Prompt Audit 等功能。

### 4.2 允许增加的内容

仅增加为高可用和分布式运行所必需的基础设施代码：

- Runtime Role；
- 生命周期管理；
- 任务启动归属；
- Redis Sentinel 客户端配置；
- PostgreSQL migration role；
- `/livez` 和 `/readyz`；
- 优雅关闭和流量 drain；
- leader lease / owner token；
- OAuth 临时 Session 的共享存储或短期 affinity；
- Refresh Token 原子消费；
- Kubernetes manifests / Helm / Kustomize；
- 必要的 claim version、lease 字段和 Outbox。

### 4.3 不在本阶段做的内容

本计划不引入：

- 独立 Billing 微服务；
- 独立 User/Identity 微服务；
- Kafka、Pulsar、NATS；
- 多租户、组织、项目模型；
- SAML、SCIM、SSO；
- 新计费功能；
- 新的用户 API；
- Service Mesh；
- 全面重做计费账本。

---

# 5. 第一阶段：增加 Runtime Role

## 5.1 新增配置

增加环境变量：

```bash
SUB2API_ROLE=all
```

支持值：

```text
all
api
worker
scheduler
migrate
bootstrap
```

默认值必须为：

```text
all
```

这样原有 Docker Compose 和裸机部署无需修改即可继续运行。

建议新增内部类型：

```go
type RuntimeRole string

const (
    RuntimeRoleAll       RuntimeRole = "all"
    RuntimeRoleAPI       RuntimeRole = "api"
    RuntimeRoleWorker    RuntimeRole = "worker"
    RuntimeRoleScheduler RuntimeRole = "scheduler"
    RuntimeRoleMigrate   RuntimeRole = "migrate"
    RuntimeRoleBootstrap RuntimeRole = "bootstrap"
)
```

建议新增文件：

```text
backend/internal/runtime/role.go
backend/internal/runtime/lifecycle.go
```

这些文件只负责：

- 读取并校验 role；
- 判断组件是否启动；
- 管理启动顺序；
- 管理停止顺序；
- 管理角色级 readiness。

不要将业务判断放到 runtime 包中。

## 5.2 修改启动入口

重点文件：

```text
backend/cmd/server/main.go
backend/cmd/server/wire.go
backend/cmd/server/wire_gen.go
backend/internal/service/wire.go
```

当前不少 Provider 在构造时直接启动后台任务：

```go
svc := NewXxxService(...)
svc.Start()
return svc
```

应改为：

```go
svc := NewXxxService(...)
return svc
```

再由角色生命周期管理器决定是否启动：

```go
switch role {
case RuntimeRoleAPI:
    app.StartAPI(ctx)
case RuntimeRoleWorker:
    app.StartWorker(ctx)
case RuntimeRoleScheduler:
    app.StartScheduler(ctx)
case RuntimeRoleAll:
    app.StartAll(ctx)
case RuntimeRoleMigrate:
    app.RunMigrations(ctx)
}
```

## 5.3 生命周期接口

建议逐步统一后台组件接口：

```go
type Component interface {
    Name() string
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}
```

不要求第一期一次性重构全部 Service。可以先为会自行启动 goroutine/ticker/cron 的组件补上显式 Start/Stop。

业务函数保持不变，例如：

```text
runScheduled
runOnePlan
SaveResult
RunCheck
CreateBackup
ProcessJob
```

变化只在于谁调用 `Start()`，以及哪个 role 可以调用它。

## 5.4 Wire 代码要求

不要长期手工修改 `wire_gen.go`。应修改：

- `backend/cmd/server/wire.go`
- `backend/internal/service/wire.go`

然后重新生成 Wire 代码。

建议最终形成：

```go
initializeAllApplication(...)
initializeAPIApplication(...)
initializeWorkerApplication(...)
initializeSchedulerApplication(...)
initializeMigrationApplication(...)
```

这些入口可以复用同一组 Repository 和 Service 构造逻辑，但根据 role 启动不同后台组件。

---

# 6. 第二阶段：API Role

## 6.1 API Role 保留的组件

API Pod 继续运行用户请求热路径：

| 模块 | 保留原因 |
|---|---|
| Gin Router / HTTP Server | 对外 API |
| Auth / JWT / API Key | 请求鉴权 |
| Gateway Service | 上游转发 |
| OpenAI / Gemini / Anthropic Handler | 协议入口 |
| Billing Eligibility | 请求前余额、订阅和配额检查 |
| Redis 并发 Lease | 跨 Pod 并发控制 |
| Scheduler Snapshot 读取 | 请求即时账号选择 |
| Sticky Session | 请求粘性 |
| User Message Queue 请求逻辑 | 请求调度语义 |
| SSE / WebSocket 运行时 | 长连接转发 |
| Usage Record Worker Pool | 保持现有请求后计量行为 |
| API Key L1 Cache Subscriber | 每个 API Pod 都需要清理自己的 L1 |
| Deferred last-used flush | 处理当前 Pod 产生的状态 |

## 6.2 API Role 不启动的组件

API Pod 不启动：

```text
ScheduledTestRunnerService
ChannelMonitorRunner
Backup cron
Prompt Audit Runner
BatchImageWorkerRuntime
UsageCleanupService
IdempotencyCleanupService
Ops Aggregation / Cleanup / Scheduled Report
全量 TokenRefresh 后台循环
AccountExpiryService
ProxyExpiryService
UserPlatformQuotaUsageFlusher
EmailQueueService
```

## 6.3 API Deployment

初始配置：

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: sub2api-api
spec:
  replicas: 3
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 0
      maxSurge: 1
```

环境变量：

```yaml
- name: SUB2API_ROLE
  value: api
- name: AUTO_SETUP
  value: "false"
- name: SKIP_SETUP
  value: "true"
```

API Pod 必须配置：

- readiness probe；
- liveness probe；
- PDB；
- topology spread；
- Pod anti-affinity；
- 合理的 `terminationGracePeriodSeconds`；
- SSE/WebSocket read timeout；
- 正确的 `trusted_proxies`。

---

# 7. 第三阶段：Worker Role

## 7.1 Worker 承担的组件

| 模块 | 现有分布式能力 | 第一阶段策略 |
|---|---|---|
| Prompt Audit Runner | PostgreSQL `SKIP LOCKED + claim_version` | 可多副本，但先单副本验证 |
| Auth Cache Invalidation Worker | `SKIP LOCKED + claimed_by` | 可多副本，但先单副本验证 |
| Batch Image Worker | Redis reserve + token lock + heartbeat | 可多副本，但先单副本验证 |
| Batch stale recovery | Redis active queue/recovery | 与 Batch Worker 同角色 |
| Usage Cleanup | 有 claim，但完成缺少 fencing | 初期单副本 |
| User Platform Quota Flusher | Redis dirty set | 初期单副本 |
| Idempotency Cleanup | 删除过期记录基本幂等 | 初期单副本 |
| Pricing refresh | 会写本地文件 | 初期单副本 |
| Email Queue | 当前为本地内存 channel | 先不从 API 拆出，改造后再移动 |
| Audit Writer | 当前为本地内存队列 | 先保持原行为，后续 durable 化 |

## 7.2 Worker 初始副本策略

第一阶段：

```yaml
replicas: 1
```

原因：当前部分后台任务虽能在同一进程内运行，但不具备完整的跨副本 claim/lease/owner fencing。

优先验证并扩展的模块：

1. Prompt Audit；
2. Auth Outbox；
3. Batch Image。

完成各自的重复执行、崩溃恢复和幂等测试后，再将 Worker 扩展到 2 副本或更多。

## 7.3 Worker Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: sub2api-worker
spec:
  replicas: 1
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 0
```

环境变量：

```yaml
- name: SUB2API_ROLE
  value: worker
```

Worker 不需要对外暴露 Service。

---

# 8. 第四阶段：Scheduler Role 和 Leader Lease

## 8.1 Scheduler 负责的组件

```text
Scheduled Test Runner
Channel Monitor Runner
Backup Scheduler
Account Expiry
Proxy Expiry
Subscription Expiry
Token Refresh 后台循环
Ops Metrics / Aggregation / Cleanup / Report / Alert
Scheduler Snapshot full rebuild
```

## 8.2 Scheduler 为什么部署两个副本

如果 Scheduler 只部署一个 Pod，虽然可以避免重复任务，但仍然存在后台单点。

建议部署：

```yaml
replicas: 2
```

但只有获得 leader lease 的 Pod 执行不具备完整 claim 的任务：

```text
Scheduler A → 获取 leader lease → 执行任务
Scheduler B → standby，不执行任务

Scheduler A 故障
  → lease 过期
  → Scheduler B 获取 lease
  → 继续加载和执行任务
```

## 8.3 Leader Lease 实现

优先复用项目已有机制：

- Redis `SETNX + TTL`；
- owner token；
- Lua compare-delete；
- Redis 不可用时 PostgreSQL advisory lock fallback。

现有参考：

```text
backend/internal/repository/leader_lock_cache.go:14-42
backend/internal/service/leader_lock.go
```

需要保证：

- 每个 Scheduler 实例使用随机 owner token；
- leader lease 定期续租；
- 失去 lease 后立即停止新一轮任务；
- 长任务期间检查 ownership；
- 释放时只能删除自己的 lease。

## 8.4 Scheduled Test 最小改造

当前执行逻辑保持不变：

```text
ListDue
→ RunTestBackground
→ SaveResult
→ UpdateAfterRun
```

只在入口增加 leader guard：

```go
func (s *ScheduledTestRunnerService) runScheduled() {
    release, acquired := s.leader.TryAcquire(
        ctx,
        "scheduled-test-runner",
        ttl,
    )
    if !acquired {
        return
    }
    defer release()

    s.runScheduledInternal()
}
```

当前相关代码：

- `backend/internal/service/scheduled_test_runner_service.go:45-147`
- `backend/internal/repository/scheduled_test_repo.go:51-85`

第二阶段再考虑给计划表增加 `claimed_by`、`claim_version`、`lease_expires_at`，实现真正的多 Scheduler 并行 claim。

## 8.5 Channel Monitor 最小改造

当前每个 Monitor 是本地 timer/goroutine：

- `backend/internal/service/channel_monitor_runner.go:115-198`
- `backend/internal/service/channel_monitor_runner.go:236-295`

第一阶段仍保留本地 timer，但只允许 Scheduler leader 启动。

leader 切换时：

```text
旧 leader 失去 lease
  → Stop ChannelMonitorRunner
  → 取消本地 timers

新 leader 获得 lease
  → Start ChannelMonitorRunner
  → 从 PostgreSQL 重新加载 enabled monitors
  → 重建本地 timers
```

这样不改变 Channel Monitor 业务逻辑，只增加运行时互斥。

## 8.6 Backup Scheduler

Backup 的 `backingUp` / `restoring` / cron 目前是进程内状态：

- `backend/internal/service/backup_service.go:118-149`
- `backend/internal/service/backup_service.go:171-192`
- `backend/internal/service/backup_service.go:383-473`
- `backend/internal/service/backup_service.go:741-753`

第一阶段要求：

- 只有 Scheduler leader 可以触发计划备份；
- Restore 不从常驻 API/Worker/Scheduler Pod 执行；
- Restore 通过人工触发 Kubernetes Job；
- Backup Job 使用 PostgreSQL/对象存储已有配置；
- 不修改备份业务格式和存储逻辑。

---

# 9. Redis Sentinel 改造

## 9.1 当前限制

当前 Redis 初始化固定为单节点客户端：

```text
backend/internal/repository/redis.go:23-53
```

配置只有：

- host；
- port；
- username；
- password；
- db；
- timeout；
- pool；
- TLS。

见：

```text
backend/internal/config/config.go:1444-1467
```

没有：

- Sentinel 地址列表；
- Master Name；
- Sentinel ACL；
- Redis Failover Client。

因此不能将普通 Redis 地址直接指向 Sentinel 26379 端口。

## 9.2 增加基础设施配置

```yaml
redis:
  mode: standalone # standalone | sentinel

  host: redis
  port: 6379

  master_name: sub2api-master
  sentinel_addrs:
    - redis-sentinel-0.redis-sentinel:26379
    - redis-sentinel-1.redis-sentinel:26379
    - redis-sentinel-2.redis-sentinel:26379

  username: ""
  password: ""
  sentinel_username: ""
  sentinel_password: ""

  enable_tls: false
```

默认保持：

```yaml
mode: standalone
```

确保既有单节点部署不受影响。

## 9.3 Redis 客户端工厂

伪代码：

```go
func InitRedis(cfg *config.Config) redis.UniversalClient {
    switch cfg.Redis.Mode {
    case "sentinel":
        return redis.NewFailoverClient(&redis.FailoverOptions{
            MasterName:       cfg.Redis.MasterName,
            SentinelAddrs:    cfg.Redis.SentinelAddrs,
            SentinelUsername: cfg.Redis.SentinelUsername,
            SentinelPassword: cfg.Redis.SentinelPassword,
            Username:         cfg.Redis.Username,
            Password:         cfg.Redis.Password,
            DB:               cfg.Redis.DB,
        })

    default:
        return redis.NewClient(&redis.Options{
            Addr:         cfg.Redis.Address(),
            Username:     cfg.Redis.Username,
            Password:     cfg.Redis.Password,
            DB:           cfg.Redis.DB,
            PoolSize:     cfg.Redis.PoolSize,
            MinIdleConns: cfg.Redis.MinIdleConns,
        })
    }
}
```

需要评估当前 Repository 是否大量依赖 `*redis.Client`。如果是，统一改为项目内部 Redis 接口或 `redis.UniversalClient`，但不改变业务调用逻辑。

## 9.4 Redis Kubernetes 资源

建议：

```text
1 个 Primary
2 个 Replica
3 个 Sentinel
```

要求：

- Redis 数据节点独立 PVC；
- Sentinel quorum 为 2；
- Redis/Sentinel 跨节点反亲和；
- Sentinel `PodDisruptionBudget.minAvailable=2`；
- Redis 只允许应用 Namespace 访问；
- AOF 或等价持久化；
- 避免会淘汰 Queue、Token、Lock 的 eviction 策略；
- 验证 failover 后 Pub/Sub 重新订阅；
- 验证 failover 后 Lua、Queue、Lease 和缓存行为。

---

# 10. PostgreSQL HA 改造

## 10.1 应用连接方式

当前应用使用单一 host/port DSN：

- `backend/internal/config/config.go:1385-1440`
- `backend/internal/repository/ent.go:38-108`

推荐使用稳定的 PostgreSQL RW Endpoint：

```yaml
database:
  host: sub2api-pg-rw
  port: 5432
  sslmode: verify-full
```

这个 Service 由 PostgreSQL Operator、Patroni、云数据库或 HAProxy 维护，始终指向当前 Primary。

不做：

- 应用层读写分离；
- 连接 read replica；
- 业务 SQL 改写；
- 应用层多主机自动发现。

## 10.2 连接池预算

数据库连接池是每个 Pod 一套。连接总量需要按所有角色计算：

```text
总连接数 = API 副本数 × API MaxOpenConns
         + Worker 副本数 × Worker MaxOpenConns
         + Scheduler 副本数 × Scheduler MaxOpenConns
         + Migration Job
         + 运维余量
```

例如：

```text
API:       3 × 30
Worker:    2 × 20
Scheduler: 2 × 15
运维余量:  20
总计:       170
```

具体数值以压测和 PostgreSQL 规格为准，不能直接沿用单机连接池配置。

建议：

- 前置 PgBouncer 或 Operator 提供的连接池；
- 设置连接最大生命周期；
- 设置连接最大空闲时间；
- failover 后滚动回收旧连接；
- 监控 `MaxOpenConns`、等待连接数和 query latency。

## 10.3 Migration Role

当前 `InitEnt` 启动时会自动执行 migration：

```text
backend/internal/repository/ent.go:67-75
```

建议增加初始化选项：

```go
type EntInitOptions struct {
    RunMigrations bool
}
```

角色行为：

| Role | `RunMigrations` |
|---|---|
| `migrate` | `true` |
| `bootstrap` | `true` |
| `api` | `false` |
| `worker` | `false` |
| `scheduler` | `false` |
| `all` | `true`，兼容旧部署 |

现有 PostgreSQL advisory lock 保留，作为额外并发保护，而不是唯一发布机制。

---

# 11. OAuth 和 Refresh Token

## 11.1 OAuth 管理 Session

当前 Claude、OpenAI、Gemini、Antigravity、Grok 的 OAuth state 和 PKCE verifier 保存在本地 map：

- `backend/internal/pkg/oauth/oauth.go:47-117`
- `backend/internal/pkg/openai/oauth.go:52-124`
- `backend/internal/service/oauth_service.go:44-170`
- `backend/internal/service/openai_oauth_service.go:17-186`

### 第一阶段最小方案

只对 OAuth 管理路由启用 Ingress Cookie Affinity：

```text
/api/v1/admin/oauth/*
/api/v1/admin/openai-oauth/*
/api/v1/admin/gemini-oauth/*
```

保证：

```text
发起 OAuth → Pod A
回调 OAuth → Pod A
```

这不改变现有 OAuth 业务功能。

### 后续方案

定义 Redis Session Store：

```go
type OAuthSessionStore interface {
    Create(ctx context.Context, id string, session OAuthSession, ttl time.Duration) error
    Consume(ctx context.Context, id string) (*OAuthSession, error)
    Delete(ctx context.Context, id string) error
}
```

`Consume` 必须是 GET + DEL 的原子 Redis Lua 操作。

## 11.2 Refresh Token 原子轮转

当前流程：

```text
GET old Refresh Token
→ 验证
→ DEL old token
→ 生成新 token
```

代码位置：

```text
backend/internal/service/auth_service.go:1575-1638
backend/internal/repository/refresh_token_cache.go:52-70
```

多 API Pod 下，两个请求可能在删除前都验证成功。

最小修复：

```lua
local value = redis.call('GET', KEYS[1])
if not value then
  return nil
end
redis.call('DEL', KEYS[1])
return value
```

业务逻辑保持不变：

```text
旧 Token 只能消费一次
消费成功后签发新 Token
```

只是将原先非原子的 GET/DEL 改为原子消费。

---

# 12. 本地文件和 DATA_DIR

## 12.1 不共享可写 DATA_DIR

不要给多个 Pod 共享同一个可写 RWX 卷。

不要共享：

```text
/app/data/config.yaml
/app/data/.installed
pricing json
文件日志
debug request body
临时文件
自更新二进制
动态 pages
```

推荐：

| 数据 | 存储 |
|---|---|
| 业务状态 | PostgreSQL HA |
| Cache / Lock / Queue / Session | Redis HA |
| 图片、备份、大对象 | S3 / R2 / GCS |
| 非敏感配置 | ConfigMap |
| 密钥 | Kubernetes Secret / External Secret |
| 日志 | stdout/stderr |
| 临时文件 | `emptyDir` |
| 固定 Pricing/Pages | 镜像或只读对象存储 |

## 12.2 Setup

当前 Setup 使用：

- `config.yaml`；
- `.installed`；
- `DATA_DIR`。

代码位置：

```text
backend/internal/setup/setup.go:42-74
backend/internal/setup/setup.go:160-179
backend/internal/setup/setup.go:348-352
```

生产 Kubernetes：

```text
AUTO_SETUP=false
SKIP_SETUP=true
```

首次初始化通过独立 `bootstrap` Job 执行，不能让多个 API Pod 同时自动 Setup。

## 12.3 日志

文件日志默认路径：

```text
/app/data/logs/sub2api.log
$DATA_DIR/logs/sub2api.log
```

相关代码：

```text
backend/internal/pkg/logger/options.go:10-104
backend/internal/pkg/logger/logger.go:323-341
```

Kubernetes 中统一使用 stdout/stderr，交给日志采集系统，不让多个 Pod 写同一个 lumberjack 文件。

## 12.4 在线更新

应用内更新会下载和 rename 当前可执行文件：

```text
backend/internal/service/update_service.go:215-299
```

Kubernetes 中禁止使用应用内 update/rollback/restart 作为发布方式，改用：

- 镜像版本；
- Deployment rollout；
- Helm/Kustomize；
- GitOps。

---

# 13. 健康检查和优雅下线

## 13.1 保留 `/health`

当前 `/health` 固定返回 200：

```text
backend/internal/server/routes/common.go:9-14
```

为了兼容现有用户和监控，不修改其语义。

## 13.2 新增 `/livez`

只判断进程是否存活，不依赖 PostgreSQL 或 Redis：

```json
{"status":"live"}
```

用于 Kubernetes liveness probe。

## 13.3 新增 `/readyz`

API Role 至少检查：

```text
配置加载成功
PostgreSQL 可 Ping/Query
Redis Primary 可 Ping
Router 已装配
核心 Gateway runtime 已启动
当前未进入 shutdown drain
```

Worker Role 检查：

```text
PostgreSQL 可访问
Redis 可访问
必要 Worker 已启动
```

Scheduler Role 检查：

```text
PostgreSQL 可访问
Redis/PG leader backend 可访问
Scheduler 生命周期已启动
```

Redis/PG 不可用时：

- `/readyz` 应返回非 200；
- `/livez` 仍保持 200；
- 防止 Kubernetes 因依赖故障产生重启风暴。

## 13.4 Shutdown 顺序

当前 HTTP Shutdown 只有 5 秒：

```text
backend/cmd/server/main.go:175-189
```

建议改为可配置：

```yaml
server:
  shutdown_timeout_seconds: 120
```

退出顺序：

```text
1. readiness 标记为 false
2. 等待 Kubernetes Endpoint 摘除
3. API 停止接受新请求
4. 等待现有短请求、SSE、WebSocket drain
5. flush Deferred/Usage 等本地状态
6. 停止后台组件
7. 释放 Redis lease
8. 关闭 Redis 和 PostgreSQL
```

Kubernetes 建议：

```yaml
terminationGracePeriodSeconds: 150
```

长连接较多时提高到 300 秒。

---

# 14. Kubernetes 部署资源

建议新增：

```text
deploy/k8s/
  base/
    configmap.yaml
    secret.example.yaml
    service.yaml
    ingress.yaml
    network-policy.yaml

  migrate/
    job.yaml

  gateway/
    deployment.yaml
    hpa.yaml
    pdb.yaml

  worker/
    deployment.yaml
    pdb.yaml

  scheduler/
    deployment.yaml
    pdb.yaml

  backup/
    cronjob.yaml

  restore/
    job-template.yaml
```

也可以使用 Helm，但第一阶段 plain manifest 或 Kustomize 更易排障。

## 14.1 API Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: sub2api-api
spec:
  replicas: 3
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 0
      maxSurge: 1
```

## 14.2 Worker Deployment

第一阶段：

```yaml
replicas: 1
```

已完成 Prompt/Auth Outbox/Batch Image 分布式验收后：

```yaml
replicas: 2
```

## 14.3 Scheduler Deployment

```yaml
replicas: 2
```

应用通过 leader lease 保证同一时刻只有一个 Scheduler 执行未 claim 化任务。

## 14.4 Migration Job

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: sub2api-migrate
spec:
  backoffLimit: 2
  template:
    spec:
      restartPolicy: OnFailure
      containers:
        - name: migrate
          image: weishaw/sub2api:<version>
          args: ["--role=migrate"]
```

发布顺序：

```text
Migration Job 成功
→ Worker 更新
→ Scheduler 更新
→ Gateway 滚动更新
```

## 14.5 其他 Kubernetes 基础设施

必须配置：

- PDB；
- Pod anti-affinity；
- topology spread；
- NetworkPolicy；
- ConfigMap/Secret；
- ServiceAccount；
- Ingress SSE/WS timeout；
- HPA；
- 资源 requests/limits；
- ServiceMonitor/PrometheusRule（如果已有监控体系）。

---

# 15. PostgreSQL 和 Redis 故障策略

## 15.1 Redis 故障

不同模块当前故障行为不同：

| 功能 | 故障行为 |
|---|---|
| Account/User 并发 slot | 通常返回错误，请求失败 |
| 等待计数 | Fail-open |
| Session Limit | Fail-open，可能超限 |
| UMQ | Fail-open |
| 普通限流 | 默认 Fail-open |
| 登录敏感限流 | Fail-close |
| Balance/Subscription | 回 PostgreSQL；PG 也失败则失败 |
| Refresh Token/Passkey | Redis-only，流程失败 |
| Batch Queue | 报错并等待恢复 |

不在本计划中统一改变这些既有故障语义，因为那会改变现有业务行为。只要求：

- Redis Sentinel 能正确切换；
- readiness 能反映 Redis 不可用；
- 对关键故障行为完成演练和记录。

## 15.2 PostgreSQL 故障

应用只连接 HA Writer Endpoint：

```text
sub2api → sub2api-pg-rw → 当前 Primary
```

不要直接将业务请求连接到只读副本。

故障恢复目标：

- 新连接可以重新建立；
- 旧连接在 lifetime 到期后被回收；
- 业务请求按照现有错误处理逻辑失败或重试；
- 不因 PG 短暂故障将所有 Pod liveness 判死。

---

# 16. 测试计划

## 16.1 单元测试

新增 Runtime Role 测试：

```text
TestRoleAllStartsAllComponents
TestRoleAPIStartsOnlyAPIComponents
TestRoleWorkerStartsOnlyWorkerComponents
TestRoleSchedulerStartsOnlySchedulerComponents
TestRoleMigrateDoesNotStartHTTP
TestLifecycleStopsStartedComponents
```

新增 Leader Lease 测试：

```text
TestOnlyOneSchedulerRuns
TestLeaderReleaseOnlyOwnsLease
TestLeaderRenewal
TestLeaderTakeoverAfterExpiry
TestOldLeaderStopsAfterOwnershipLost
```

新增 Redis Sentinel 配置测试：

```text
TestStandaloneRedisConfigUnchanged
TestSentinelRedisConfig
TestInvalidSentinelConfig
```

新增 Refresh Token 原子消费测试：

```text
TestOnlyOneRefreshConsumesOldToken
TestRefreshTokenReuseRejected
```

## 16.2 集成测试

### API 多副本

启动：

```text
2 个 API Pod
1 个 Worker Pod
1 个 Scheduler Pod
PostgreSQL
Redis
```

验证：

- API Key 鉴权；
- 登录/刷新；
- 普通 Gateway 请求；
- SSE；
- WebSocket；
- Sticky Session；
- Account/User/API Key 并发；
- Usage/Billing；
- API Key cache invalidation；
- 管理配置修改后的缓存收敛。

### Worker 故障

- Worker 处理中被杀；
- Batch Image active task recovery；
- Prompt Audit lease recovery；
- Auth Outbox claim recovery；
- Usage Cleanup 任务恢复。

### Scheduler 故障

- Leader Pod 被杀；
- Standby 获得 lease；
- Scheduled Test 不重复执行；
- Channel Monitor 不重复探测；
- Backup 不并发执行。

### Redis Sentinel 故障

- Primary failover；
- Sentinel 节点故障一个；
- 应用连接重建；
- Pub/Sub 重订阅；
- Queue、Lease、Cache、Rate Limit 验证。

### PostgreSQL HA 故障

- Primary failover；
- 旧连接回收；
- 新请求恢复；
- Migration Job 状态保持；
- 计费去重和事务一致性验证。

## 16.3 回归测试

必须确保 `ROLE=all` 与改造前行为一致：

- 现有 Backend Go tests；
- 现有 Frontend tests；
- 现有 API integration tests；
- OAuth tests；
- Billing tests；
- Batch Image tests；
- Prompt Audit tests；
- Redis tests；
- migration tests。

---

# 17. 分阶段实施路线

## 阶段 0：基线冻结

工作项：

1. 固定当前生产版本；
2. 记录 PostgreSQL migration revision；
3. 备份 PostgreSQL 并验证恢复；
4. 确认 Redis 持久化；
5. 收集 QPS、SSE/WS 数、PG 连接、Redis pool、worker 任务耗时；
6. 固定 `JWT_SECRET` 和 `TOTP_ENCRYPTION_KEY`；
7. 确认当前 `ROLE=all` 单体回归测试通过。

验收：

```text
能够恢复现有数据并启动
功能基线完整
明确 PG/Redis 容量预算
```

## 阶段 1：Role 化

工作项：

1. 新增 `SUB2API_ROLE`；
2. 默认保持 `all`；
3. Provider 构造和 Start 解耦；
4. API/Worker/Scheduler 生命周期管理；
5. 新增 `/livez`、`/readyz`；
6. 增加 graceful shutdown timeout；
7. 角色启动清单测试。

验收：

| 场景 | 预期 |
|---|---|
| `ROLE=all` | 与改造前一致 |
| `ROLE=api` | API 正常，不启动后台任务 |
| `ROLE=worker` | 不监听 HTTP，Worker 正常 |
| `ROLE=scheduler` | 不监听 HTTP，仅运行 Scheduler |
| `ROLE=migrate` | 执行迁移后退出 |
| SIGTERM | 正确停止，不遗留任务 |

## 阶段 2：外置 HA 数据面

工作项：

1. Redis Sentinel client；
2. Redis Primary/Replica/Sentinel；
3. PostgreSQL HA RW Endpoint；
4. API/Worker/Scheduler 禁用自动 migration；
5. Migration Job 接管 DDL；
6. Bootstrap Job 接管首次初始化；
7. 不共享可写 DATA_DIR。

验收：

```text
Redis failover 后连接恢复
PostgreSQL failover 后连接恢复
Migration Job 可重复执行
```

## 阶段 3：Gateway 多副本

工作项：

1. Gateway `replicas=2`；
2. Ingress、PDB、topology spread；
3. OAuth 管理路由 affinity；
4. Refresh Token 原子消费；
5. Readiness；
6. HPA；
7. 压测 HTTP、SSE、WS、鉴权、计费、支付回调。

验收：

```text
删除一个 Gateway Pod，新请求继续成功
API Key/Auth/并发/计费无重复或明显错误
```

## 阶段 4：Worker/Scheduler 高可用

工作项：

1. Scheduler Leader Lease；
2. Scheduled Test leader guard；
3. Channel Monitor leader guard；
4. Backup leader guard；
5. Prompt/Auth Outbox/Batch Image 扩至 2 Worker；
6. Usage Cleanup claim fencing；
7. OAuth Refresh/Scheduler Bucket/Redeem owner token；
8. Channel Cache 跨 Pod invalidation；
9. Email 改为 durable Outbox/Job。

验收：

```text
删除 Scheduler Leader，备用 Scheduler 接管
删除 Worker，其他 Worker 可继续处理任务
不出现重复测试、重复监控、重复备份
```

---

# 18. 回滚策略

## 18.1 保留 `ROLE=all`

第一期必须保留：

```bash
SUB2API_ROLE=all
```

作为单机兼容模式和紧急恢复路径。

## 18.2 角色拆分异常时的回滚

```text
1. Worker Deployment 缩容到 0
2. Scheduler Deployment 缩容到 0
3. Gateway 从 Service 摘除或缩容到 0
4. 启动一个同版本 ROLE=all Pod
5. 使用同一份 ConfigMap 和 Secret
6. 连接同一 PostgreSQL HA 和 Redis HA
7. 通过 /readyz、登录和 Gateway 冒烟测试
8. 恢复流量
```

不能让 `ROLE=all` 与 Worker/Scheduler 长期并行，否则会重复执行后台任务。

## 18.3 数据库回滚原则

```text
应用镜像可以回滚
数据库 migration 不直接回滚
```

数据库变更遵循：

```text
expand
→ 发布兼容代码
→ 稳定运行
→ contract
```

第一阶段尽量不做业务 schema 变化。后续增加 claim/lease 字段时，先保证旧代码可以正常读取和忽略新字段。

---

# 19. 最小完成标准

以下全部满足后，可以认为项目具备保留功能的 Kubernetes 高可用基础：

```text
[ ] Gateway 至少 2 个 Pod，任一 Pod 故障不影响新请求
[ ] PostgreSQL 通过稳定 RW HA Endpoint 访问
[ ] Redis 通过 Sentinel 发现 Primary，或稳定 Primary Proxy
[ ] Migration 仅由 Kubernetes Job 执行
[ ] API Pod 不运行重复 cron/worker
[ ] Scheduler 具备 leader standby 接管能力
[ ] 已验证的 Worker 任务可以 2 副本运行
[ ] OAuth 管理路由具备 affinity 或 Redis Session Store
[ ] Refresh Token 轮转是原子操作
[ ] 所有 Pod 使用一致的 JWT/TOTP/DB/Redis Secret
[ ] 不共享可写 DATA_DIR
[ ] /readyz、PDB、优雅下线和故障演练完成
[ ] ROLE=all 可作为紧急恢复模式
```

---

# 20. 最终结论

最适合当前项目的改造路线是：

```text
当前单体
  ↓
增加 Runtime Role 和生命周期管理
  ↓
同一镜像按 api/worker/scheduler/migrate 运行
  ↓
Gateway 多副本
  ↓
Worker 和 Scheduler 逐步高可用
  ↓
PostgreSQL HA + Redis Sentinel + Kubernetes
```

本计划不会把原有业务拆成复杂的微服务网格，也不会让 API Pod 与 Worker Pod 之间增加新的同步 RPC。

同步请求链路继续在 API Pod 内保留原有函数调用；异步任务继续通过 PostgreSQL、Redis、Queue 和 Outbox 协作。

首期最关键的代码改动集中在：

```text
启动入口
Wire 装配
生命周期管理
后台任务归属
Scheduler Leader Lease
Redis Sentinel Client
Migration Role
Health/Readiness
Graceful Shutdown
Kubernetes 部署清单
```

首期最关键的部署目标是：

```text
API 高可用
后台任务不重复
数据库和 Redis 使用 HA
现有功能行为不变
单机部署仍可运行
```
