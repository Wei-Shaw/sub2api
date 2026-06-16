# V5 Designer — 通用集成能力 SDK 化 · 详细设计 + 迁移剧本

> 作用：把 V5-CURATE 敲定的 5 项能力（W1-W5）+ 2 个迁移功能（W6-W9）的接口、proto、代码骨架、跨进程时序、step 级任务图、业界参考全部钉死。Implementer sub-agent 看完即可分头开工，不再做架构判断。
> 设计原则（用户原话，最高优先级）：**集成能力应该抽象、解耦、复用、统一，不要专门为了渠道功能定制**。proto / SDK / capability 字符串里不出现 channel 关键字。
> base：`9fb6a6a2 Merge V5 curator: capability scope + 11-item task graph (4 waves)`（`feat/plugin-system-fixes`）
> 子分支：`feat/plugin-system-fixes--v5-design`

---

## 0. Curator 留下的 3 个待决问题 — 最终决策

| # | 问题 | 决策 | 理由（一行） |
|---|------|------|-------------|
| D1 | W8 走 SQLProxy 自重写 mapping ∪ pricing，还是把 `HostServiceProxyCapability` 提升到 V5.5？ | **W8 走 SQLProxy 自重写 + 把 170 行 mapping ∪ pricing 抽成 plugin 内 `domain/channel_view.go`，并在 §6.2 给出 "snapshot copy" 模式（plugin 启动时把 host service 的纯逻辑 vendor 进来）** | 提升 V5.5 会让本期落地周期 +3-5 day；且当前 host 的 `Channel.SupportedModels()` 只在两处用，是无状态纯函数，复制成本可控；HostServiceProxy 留作 V6 第一项（已写入 §10） |
| D2 | W3 admin 表单渲染选哪个 vue-json-schema-form 实现？ | **不用 `@koumoul/vuetify-jsonschema-form` / `MaciejDybowski/vue3-schema-forms`（强依赖 Vuetify，与项目 Element Plus 冲突）；改用 `crickford/vue-json-schema-form`（明确支持 Draft-07 + 组件无关），失败兜底用手写 `<DynamicSettingsForm/>`（Vue 3 + ajv，仅 string/number/boolean/enum/object 嵌套，约 200 行）** | 项目前端是 Vue 3 + Element Plus；crickford 版本组件无关、可适配 Element Plus；**Implementer 须先做 30 分钟 PoC**，PoC 不通则走手写 fallback |
| D3 | W2 cron 库选 `robfig/cron/v3` 还是 `go-co-op/gocron`？ | **`robfig/cron/v3 v3.0.1`**（host 端 scheduler）+ 自管 leader lock 复用 `OpsCleanupService.tryAcquireLeaderLock` | gocron v2 的 `WithDistributedLocker` 与 host 现有 leader lock 体系（Redis SetNX + DB advisory fallback，见 `ops_cleanup_service.go:308-345`）会撞两套；robfig/cron 已是 gocron 内部依赖，引入它不增加依赖图深度；ticker / fixed-delay 触发不靠 cron 库（host 自己用 `time.Timer`），cron 库只解析表达式 + 算下次触发点 |

> **Implementer Sanity Check（开工前必做）**：
> 1. `git log -1 --oneline` 应是 `9fb6a6a2 Merge V5 curator`
> 2. 跑 `cd backend && go build ./... && cd ../plugin-sdk && go build ./...`，确认 base 编译干净
> 3. 跑 `wc -l plugin-sdk/proto/sdk.proto` 应 ≈ 284，避免拿到旧版本
> 4. 跑 `grep -r "ChannelMonitor" backend/internal | head -5` 应 0 命中（V1 host-clean 已删过；W6 需新建在 plugin 端）
> 5. 跑 `git show 09fd83ab:backend/internal/service/channel_monitor_runner.go | wc -l` 应 ≈ 291（取代码源 commit 仍在）
> 6. 跑 `grep -n "plugin declared migrations" backend/internal/plugin/manager.go` 应命中第 ~702 行 — W1.3 替换的精确位置

---

## 1. W1 — MigrationRunnerCapability

### 1.1 命名 + 常量

```go
// plugin-sdk/manifest.go (追加)
const CapabilityMigrationRunner = "migration_runner"
```

无需 plugin 显式 opt-in 申请：host 在 `manifest.GetMigrations()` 非空时自动激活 — 与 capability allow-list 设计一致（任何**写表**操作都隐含审批，所以保留 capability 字符串供 V6 收紧用）。

### 1.2 完整 proto 定义

```proto
// plugin-sdk/proto/plugin.proto 改动：PluginLifecycle 加 GetMigration RPC + ManifestResponse 加 migrations

service PluginLifecycle {
  // ... 已有 5 个 RPC ...
  rpc GetMigration(GetMigrationRequest) returns (GetMigrationResponse);
}

message GetMigrationRequest  { string filename = 1; }
message GetMigrationResponse {
  string filename          = 1;
  bytes  sql               = 2;   // bytes 而非 string，保留 BOM/编码原貌
  string checksum_sha256   = 3;   // hex；SDK 端在打包时算好
  bool   non_transactional = 4;   // 含 CREATE INDEX CONCURRENTLY 等需独立 tx
}

// ManifestResponse 扩展：保留 migration_files (deprecated) + 新增 migrations
message ManifestResponse {
  // ... 已有字段 1-40 ...
  // DEPRECATED: 旧字段，新 plugin 应使用 migrations。host 收到非空 migrations 时忽略此字段。
  // repeated string migration_files = 30;
  repeated MigrationDecl migrations = 41;
}

message MigrationDecl {
  string filename          = 1;   // 例 "001_create_monitor.sql"，按字典序应用
  string checksum_sha256   = 2;   // hex
  bool   non_transactional = 3;
}
```

### 1.3 SDK Go 接口签名

```go
// plugin-sdk/migration.go (新增)
package pluginsdk

import (
    "crypto/sha256"
    "embed"
    "encoding/hex"
    "fmt"
    "io/fs"
    "sort"
    "strings"
    "sync"
)

// MigrationFile 是 SDK 内部的 migration 表示。
type MigrationFile struct {
    Filename         string
    SQL              []byte
    ChecksumSHA256   string
    NonTransactional bool
}

// MigrationProvider 由 plugin 实现，告诉 SDK migration 内容来源。
// 大部分 plugin 直接用 NewEmbedMigrationProvider 即可。
type MigrationProvider interface {
    ListMigrations() ([]MigrationFile, error)
}

// NewEmbedMigrationProvider 从 embed.FS 读取目录下所有 .sql 文件，
// 按 filename 字典序返回。SQL 文件首行若是 `-- non_transactional`
// 则该 migration 标记为非事务型（host 在 advisory lock 外单独跑）。
func NewEmbedMigrationProvider(efs embed.FS, root string) MigrationProvider {
    return &embedProvider{efs: efs, root: root}
}

// RegisterMigrationProvider 在 Manifest 阶段把 provider 注册进 SDK runner。
// 后续 host 调用 GetManifest 时 SDK 会自动把 MigrationDecl 列表填入响应。
func RegisterMigrationProvider(p MigrationProvider) error
```

### 1.4 SDK 客户端实现要点（伪代码，关键 30 行）

```go
// plugin-sdk/migration.go (实现)
type embedProvider struct {
    efs   embed.FS
    root  string
    once  sync.Once
    cache []MigrationFile
    err   error
}

func (p *embedProvider) ListMigrations() ([]MigrationFile, error) {
    p.once.Do(func() {
        entries, err := fs.ReadDir(p.efs, p.root)
        if err != nil { p.err = err; return }
        out := make([]MigrationFile, 0, len(entries))
        for _, e := range entries {
            if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") { continue }
            data, err := fs.ReadFile(p.efs, p.root+"/"+e.Name())
            if err != nil { p.err = fmt.Errorf("read %s: %w", e.Name(), err); return }
            sum := sha256.Sum256(data)
            nonTx := strings.HasPrefix(strings.TrimSpace(string(data)), "-- non_transactional")
            out = append(out, MigrationFile{
                Filename: e.Name(), SQL: data,
                ChecksumSHA256:  hex.EncodeToString(sum[:]),
                NonTransactional: nonTx,
            })
        }
        sort.Slice(out, func(i, j int) bool { return out[i].Filename < out[j].Filename })
        p.cache = out
    })
    return p.cache, p.err
}

// GetMigration RPC handler (在 plugin 进程的 PluginLifecycle server 端)
func (s *lifecycleServer) GetMigration(ctx context.Context, req *pb.GetMigrationRequest) (*pb.GetMigrationResponse, error) {
    files, err := s.migrationProvider.ListMigrations()
    if err != nil { return nil, err }
    for _, f := range files {
        if f.Filename == req.Filename {
            return &pb.GetMigrationResponse{
                Filename: f.Filename, Sql: f.SQL,
                ChecksumSha256:   f.ChecksumSHA256,
                NonTransactional: f.NonTransactional,
            }, nil
        }
    }
    return nil, status.Errorf(codes.NotFound, "migration %q not declared", req.Filename)
}
```

### 1.5 Host 服务端实现要点

**新增文件**：无（**复用** `backend/internal/plugin/migrations.go` 的 `RunPluginMigrations` + `pluginMigrationLockID`）。

**改动文件**：`backend/internal/plugin/manager.go` 第 700-705 行（当前只 log "plugin declared migrations"），替换为真实拉 SQL + 调 `RunPluginMigrations`：

```go
// manager.go startInstance() 中替换 696-705 行
if decls := manifest.GetMigrations(); len(decls) > 0 {
    files, err := m.fetchPluginMigrations(ctxInit, lifecycle, inst.Name, decls)
    if err != nil {
        m.transitionError(inst, fmt.Errorf("fetch migrations: %w", err))
        return err  // plugin 启动失败（Curator Q3 决策）
    }
    if err := RunPluginMigrations(ctxInit, m.coreDB, inst.Name, files); err != nil {
        m.transitionError(inst, fmt.Errorf("apply migrations: %w", err))
        return err
    }
    m.logger.Info("plugin migrations applied",
        "plugin", inst.Name, "count", len(files),
    )
}

// 新增 helper（约 25 行）
func (m *PluginManager) fetchPluginMigrations(
    ctx context.Context, lc pb.PluginLifecycleClient,
    pluginName string, decls []*pb.MigrationDecl,
) ([]MigrationFile, error) {
    out := make([]MigrationFile, 0, len(decls))
    for _, d := range decls {
        resp, err := lc.GetMigration(ctx, &pb.GetMigrationRequest{Filename: d.Filename})
        if err != nil {
            return nil, fmt.Errorf("get migration %s: %w", d.Filename, err)
        }
        sum := sha256.Sum256(resp.Sql)
        if hex.EncodeToString(sum[:]) != d.ChecksumSha256 {
            return nil, fmt.Errorf("migration %s checksum drift: manifest=%s, content=%x",
                d.Filename, d.ChecksumSha256, sum[:])
        }
        out = append(out, MigrationFile{
            Filename: d.Filename, Content: resp.Sql,
        })
    }
    return out, nil
}
```

> 现有 `RunPluginMigrations` 已实现 advisory lock + plugin_migrations 表 + checksum 校验，无需改动；只需在 plugin 调用前**先拉 SQL 内容并校验 manifest checksum**。

### 1.6 失败/降级具体策略

| 场景 | 行为 | 数字 |
|------|------|-----|
| `lc.GetMigration` RPC 超时 | 重试 3 次（间隔 200ms / 600ms / 2s），仍失败 → plugin 启动失败 | 单次 RPC timeout = 5s，总预算 ≤ 15s |
| Manifest checksum 与拉到的内容不一致 | 立即 fail，不重试（防 plugin 进程被替换） | 0 重试 |
| `RunPluginMigrations` 拿不到 advisory lock | 已有 `pgPluginAdvisoryLock` 内 500ms 重试循环；外层 ctx timeout = 30s，超时 fail | 30s |
| 单条 migration SQL 执行失败 | 回滚事务（`RunPluginMigrations` 已实现），plugin 启动失败 | 单 SQL timeout = 60s，整批预算 5min |
| `non_transactional=true` migration | host 在事务外用独立 conn 跑（CREATE INDEX CONCURRENTLY 不支持 BEGIN） | 单条 timeout = 5min |
| 运维 escape hatch | `plugins.<name>.skip_migration=true` 配置项；进 manager 后跳过 fetchPluginMigrations | — |

### 1.7 capability 声明 + opt-in

```go
// plugin manifest 内：
return &pluginsdk.Manifest{
    // ...
    Capabilities: []string{pluginsdk.CapabilityMigrationRunner},
}
```

host `manager.go::approveCapabilities()` 中**默认 allow-list** 加入 `CapabilityMigrationRunner`（与 `CapabilityRedisRawKeys` 类似，但前者所有 plugin 都默许，因为 plugin 命名空间隔离已由 `plugin_migrations` 表的 `(plugin_name, filename)` 主键保证）。

### 1.8 跨进程边界图

```
plugin                                              host (manager.go startInstance)
  │                                                  │
  │── GetManifest() ─────────────────────────────────▶│
  │◀─ ManifestResponse{migrations:[                  │
  │     {001..., chk=abc..., non_tx=false},          │
  │     {002..., chk=def..., non_tx=true}]}          │
  │                                                  │
  │                                                  ├─ RunPluginMigrations(ctx, db, "channel-monitor", []MigrationFile{})
  │                                                  ├─    pgPluginAdvisoryLock("plugin:channel-monitor")
  │                                                  │
  │── GetMigration("001_...sql") ◀───────────────────│
  │── reply{sql=..., chk=abc...} ────────────────────▶│
  │                                                  ├─    SHA256(sql) == chk? yes
  │                                                  ├─    BEGIN; exec sql; INSERT plugin_migrations; COMMIT
  │── GetMigration("002_...sql") ◀───────────────────│
  │── reply{sql=..., chk=def..., non_tx=true} ──────▶│
  │                                                  ├─    SHA256 ok
  │                                                  ├─    (skip BEGIN); exec sql; INSERT plugin_migrations
  │                                                  ├─    pgPluginAdvisoryUnlock
  │                                                  ▼
  │                                            transition StateRunning
```

### 1.9 业界参考（W1）

1. **HashiCorp Nomad plugin migrations** ([github.com/hashicorp/go-plugin](https://github.com/hashicorp/go-plugin)) — Nomad 的 task driver plugin 通过 gRPC `Capabilities` RPC 声明能力；migration 由 host 统一 apply，与本设计同源。
2. **Atlas Cloud plugin-driven migrations** ([atlasgo.io](https://atlasgo.io)) — 采用其 "host 拥有事务 + advisory lock，plugin 只声明 SQL" 边界。
3. **pg-safe-migrate** ([dev.to](https://dev.to/defnotwig/pg-safe-migrate-stop-shipping-unsafe-postgres-migrations-1m5n)) — `non_transactional` 自动检测 + advisory lock 三件套灵感来源。

---

## 2. W2 — JobSchedulerCapability

### 2.1 命名 + 常量

```go
const CapabilityJobScheduler = "job_scheduler"
```

显式 capability — 因为 leader-only job 跨节点协调，host 必须知道哪些 plugin 用到才能预留 lock 资源。

### 2.2 完整 proto 定义

```proto
// plugin-sdk/proto/sdk.proto 追加

service JobScheduler {
  // 双向 stream：plugin 启动时 send Register；host 收到后开始按 spec 推 Trigger；
  // plugin 收到 Trigger → 跑 handler → send Ack；admin Run-now 通过 ManualTrigger 走相同链路。
  rpc Subscribe(stream JobMessage) returns (stream JobTrigger);
}

message JobMessage {
  oneof msg {
    JobRegistration register = 1;
    JobAck          ack      = 2;
    ManualTrigger   manual   = 3;
  }
}

message JobRegistration { repeated JobSpec specs = 1; }

message JobSpec {
  string name              = 1;
  string kind              = 2;   // "interval" | "cron" | "fixed_delay"
  int64  interval_nanos    = 3;
  string cron_spec         = 4;   // 标准 5/6 字段
  int64  fixed_delay_nanos = 5;
  bool   leader_only       = 6;
  int32  concurrency       = 7;   // 同名 trigger 同时跑实例数上限，默认 1
  int64  timeout_nanos     = 8;   // 单次 handler 最大耗时，到点取消 ctx
}

message JobTrigger {
  string job_name            = 1;
  string trigger_id          = 2;   // host 生成 ULID，用于 ack 关联
  int64  fire_time_unix_nano = 3;
  bool   manual              = 4;
}

message JobAck {
  string trigger_id     = 1;
  bool   success        = 2;
  string error          = 3;   // success=false 时填写
  int64  duration_nanos = 4;
}

message ManualTrigger { string job_name = 1; }
```

### 2.3 SDK Go 接口签名

```go
// plugin-sdk/jobs.go (新增)

type JobTriggerKind string
const (
    TriggerInterval   JobTriggerKind = "interval"
    TriggerCron       JobTriggerKind = "cron"
    TriggerFixedDelay JobTriggerKind = "fixed_delay"
)

type JobSpec struct {
    Name        string
    Trigger     JobTrigger
    LeaderOnly  bool
    Concurrency int           // 默认 1；≤0 视为 1
    Timeout     time.Duration // 默认 5min；handler ctx 在到点时 cancel
}

type JobTrigger struct {
    Kind       JobTriggerKind
    Interval   time.Duration // Kind=interval
    CronSpec   string        // Kind=cron，6 字段（带秒）或 5 字段
    FixedDelay time.Duration // Kind=fixed_delay
}

type JobHandler func(ctx context.Context, jobName string) error

type JobsClient interface {
    Register(spec JobSpec, h JobHandler) error
    // TriggerLocal 只在 plugin 进程内手动触发（admin "Run now" 走 host → ManualTrigger 回流）。
    // 直接调用 = 等价 ManualTrigger 但跳过 host leader_only 校验，仅供集成测试用。
    TriggerLocal(name string) error
}

// 注册入口（在 PluginContext 上）
func (ctx *PluginContext) Jobs() JobsClient
```

### 2.4 SDK 客户端实现要点（伪代码，关键 60 行）

```go
// plugin-sdk/jobs.go
type jobsClient struct {
    stream    pb.JobScheduler_SubscribeClient
    handlers  sync.Map   // name -> JobHandler
    specs     []*pb.JobSpec
    inFlight  sync.Map   // name -> chan struct{} (concurrency semaphore)
    sendMu    sync.Mutex
    logger    *slog.Logger
}

func (c *jobsClient) Register(spec JobSpec, h JobHandler) error {
    pbSpec := convertSpec(spec)
    c.specs = append(c.specs, pbSpec)
    c.handlers.Store(spec.Name, h)
    if spec.Concurrency <= 0 { spec.Concurrency = 1 }
    sem := make(chan struct{}, spec.Concurrency)
    c.inFlight.Store(spec.Name, sem)
    return nil
}

// runJobLoop 在 SDK runner Init 阶段启动；与 LogProxy 同样的"client streaming + reconnect"范式
func (c *jobsClient) runJobLoop(ctx context.Context, client pb.JobSchedulerClient) {
    backoff := 1 * time.Second
    const maxBackoff = 30 * time.Second
    for {
        if ctx.Err() != nil { return }
        stream, err := client.Subscribe(ctx)
        if err != nil { c.sleep(ctx, backoff); backoff = min2(backoff*3, maxBackoff); continue }
        c.stream = stream
        // 1. 第一帧 Register
        if err := stream.Send(&pb.JobMessage{Msg: &pb.JobMessage_Register{
            Register: &pb.JobRegistration{Specs: c.specs},
        }}); err != nil { _ = stream.CloseSend(); c.sleep(ctx, backoff); continue }
        backoff = 1 * time.Second
        // 2. 接收 Trigger 循环
        for {
            trig, err := stream.Recv()
            if err != nil {
                c.logger.Warn("job stream broken, reconnecting", "err", err)
                break
            }
            go c.dispatch(ctx, trig)  // dispatch 内拿 semaphore + run handler + send Ack
        }
    }
}

func (c *jobsClient) dispatch(ctx context.Context, trig *pb.JobTrigger) {
    h, ok := c.handlers.Load(trig.JobName)
    if !ok { c.sendAck(trig.TriggerId, false, "no handler", 0); return }
    semVal, _ := c.inFlight.Load(trig.JobName)
    sem := semVal.(chan struct{})
    select {
    case sem <- struct{}{}:
        defer func() { <-sem }()
    default:
        c.sendAck(trig.TriggerId, false, "concurrency limit reached", 0)
        return
    }
    spec := c.findSpec(trig.JobName)
    runCtx, cancel := context.WithTimeout(ctx, time.Duration(spec.TimeoutNanos))
    defer cancel()
    start := time.Now()
    err := h.(JobHandler)(runCtx, trig.JobName)
    dur := time.Since(start)
    if err != nil {
        c.sendAck(trig.TriggerId, false, err.Error(), dur.Nanoseconds())
        return
    }
    c.sendAck(trig.TriggerId, true, "", dur.Nanoseconds())
}
```

### 2.5 Host 服务端实现要点

**新增文件**：`backend/internal/plugin/job_scheduler_server.go`（~250 行）。

```go
package plugin

// JobSchedulerServer 实现 pb.JobSchedulerServer。
// 设计：每个 plugin 一个 PluginScheduler（含 cron.Cron + ticker map），所有 PluginScheduler
// 共享 leaderLockProvider（复用 OpsCleanupService.tryAcquireLeaderLock 模式）。
type JobSchedulerServer struct {
    pb.UnimplementedJobSchedulerServer
    resolver   func(stream pb.JobScheduler_SubscribeServer) string
    leaderLock LeaderLockProvider // 接口，注入 OpsCleanupService 或测试桩
    schedulers sync.Map           // pluginName -> *PluginScheduler
    history    JobHistoryRecorder // 接口：UpsertJobRun(name, trigger_id, success, dur, err)
    logger     *zap.Logger
}

type LeaderLockProvider interface {
    TryAcquire(ctx context.Context, key string) (release func(), isLeader bool)
}

type PluginScheduler struct {
    pluginName string
    cron       *cron.Cron
    intervals  map[string]*time.Ticker
    pending    sync.Map           // trigger_id -> *triggerCtx
    sendCh     chan *pb.JobTrigger
    leaderLock LeaderLockProvider
    leaderOnly map[string]bool
    logger     *zap.Logger
}

func (s *JobSchedulerServer) Subscribe(stream pb.JobScheduler_SubscribeServer) error {
    pluginName := s.resolver(stream)
    if pluginName == "" { return status.Error(codes.Unauthenticated, "missing caller identity") }

    // 1. 等 plugin 第一帧 Register
    first, err := stream.Recv()
    if err != nil { return err }
    reg := first.GetRegister()
    if reg == nil { return status.Error(codes.InvalidArgument, "first message must be Register") }

    ps := newPluginScheduler(pluginName, s.leaderLock, s.logger)
    if err := ps.applySpecs(reg.Specs); err != nil { return err }
    s.schedulers.Store(pluginName, ps)
    defer s.schedulers.Delete(pluginName)
    ps.start()
    defer ps.stop()

    // 2. 双向 fan-in/fan-out
    go s.recvLoop(stream, ps)  // 收 Ack + ManualTrigger
    return s.sendLoop(stream, ps)  // 发 Trigger
}

// 关键 dispatch（cron 触发 + leader_only 检查）
func (ps *PluginScheduler) onCronFire(jobName string) {
    if ps.leaderOnly[jobName] {
        release, isLeader := ps.leaderLock.TryAcquire(context.Background(), "plugin-job:"+ps.pluginName+":"+jobName)
        if !isLeader { return }
        defer release()
    }
    triggerID := ulid.Make().String()
    ps.pending.Store(triggerID, &triggerCtx{firedAt: time.Now()})
    ps.sendCh <- &pb.JobTrigger{
        JobName: jobName, TriggerId: triggerID,
        FireTimeUnixNano: time.Now().UnixNano(),
    }
}
```

**新增 admin handler**：`POST /api/v1/admin/plugins/:name/jobs/:job/trigger` → 调 `JobSchedulerServer.ManualFire(pluginName, jobName)` → server 把 `ManualTrigger` 通过下一个 sendLoop tick push 给 plugin。

**改 wire.go**：注入 `JobSchedulerServer` 到 SDK gRPC server；`leaderLock` 注入 `OpsCleanupService` 适配器（暴露其 `tryAcquireLeaderLock` 为 `LeaderLockProvider.TryAcquire`）。

### 2.6 失败/降级具体策略

| 场景 | 行为 | 数字 |
|------|------|-----|
| plugin stream 断开 | host 标记该 plugin 所有 spec inactive；下次 reconnect 自动续订 | 检测：30s 心跳；reconnect 退避 1s/3s/9s/max 30s |
| plugin handler 返回 error | 写入 `plugin_job_history` 表；不自动 retry（V6+ 议题） | — |
| handler 超时（默认 5min） | host 取消 ctx；plugin 端的 dispatch 拿不到 ack 也无影响；history 记 timeout | 默认 5min，可 spec 内调整 |
| `leader_only=true` 但本节点不是 leader | host 跳过这次触发（不 send Trigger） | — |
| plugin 启动 Register 时 cron spec 解析失败 | server 返回 `InvalidArgument`，plugin 收到 stream EOF → 启动失败 | — |
| 同名 spec 已在跑 + concurrency 已满 | plugin SDK 直接 ack `success=false, error="concurrency limit reached"`，host 记一行 history | — |
| host 进程重启 | leader lock 因 TTL=30s 自动过期；plugin 重新 Subscribe；丢失的触发 = 至多 30s | TTL 30s |

### 2.7 capability 声明 + opt-in

`Capabilities: []string{pluginsdk.CapabilityJobScheduler}` — host approveCapabilities 默认放行（与 `CapabilityRedisRawKeys` 不同，job scheduling 不涉及跨 plugin 资源访问，仅需要 leader lock 协调，不算特权）。

### 2.8 跨进程边界图

```
plugin                                  host (JobSchedulerServer)
  │                                       │
  │── Subscribe(stream) ─────────────────▶│  resolver → pluginName="channel-monitor"
  │                                       │
  │── Send(Register{specs:[              │
  │     {name:"monitor.run",             │
  │      kind:"interval",interval:60s,   │
  │      concurrency:5},                 │
  │     {name:"monitor.daily-rollup",    │
  │      kind:"cron",cron:"0 2 * * *",   │
  │      leader_only:true}]}) ──────────▶│  ps.applySpecs
  │                                       │  cron.AddFunc("0 2 * * *", onCronFire)
  │                                       │  time.NewTicker(60s) → onIntervalFire
  │                                       │
  │                              ────────────── 60s 后 ──────────────
  │                                       │  onIntervalFire("monitor.run")
  │◀─ Recv() = JobTrigger{                │  triggerID=01HXX..., fire_time=now
  │     name:"monitor.run", id:01HXX...} ◀┤  pending[01HXX]=ctx
  │                                       │
  │── go dispatch(trig) ──┐               │
  │   sem ← (容量 5)       │               │
  │   handler(ctx, "monitor.run")         │
  │   ←─ 38ms 后 nil ─────┘               │
  │── Send(Ack{id:01HXX, success:true,    │
  │            duration:38_000_000}) ────▶│  pending.Delete; history.Upsert(success)
  │                                       │
  │             ─────── 02:00 UTC，leader 节点 ───────
  │                                       │  onCronFire("monitor.daily-rollup")
  │                                       │  leader_only? → tryAcquire("plugin-job:channel-monitor:monitor.daily-rollup")
  │                                       │    isLeader=true → release defer
  │◀─ Recv() = JobTrigger{daily-rollup} ◀┤  send
  │── handler 跑 1.2s ────────────────────▶│
  │── Send(Ack{success:true,1_200_000_000})▶│
```

### 2.9 业界参考（W2）

1. **River** ([github.com/riverqueue/river](https://github.com/riverqueue/river)) — work function 注册 + 周期/cron 触发分离，对应 plugin=worker / host=scheduler。
2. **Asynq** ([github.com/hibiken/asynq](https://github.com/hibiken/asynq)) — leader 单例调度 + worker 多实例消费模型。
3. **Temporal** — Worker 注册 activity 类型；scheduler 在 server 端，activity 在 worker 端跑，与本设计同构。

---

## 3. W3 — SettingsExtensionCapability

### 3.1 命名 + 常量

```go
const CapabilitySettingsExtension = "settings_extension"
```

### 3.2 完整 proto 定义

```proto
service SettingsExtension {
  rpc Get   (SettingsGetRequest)   returns (SettingsGetResponse);
  rpc Watch (SettingsWatchRequest) returns (stream SettingsChangeEvent);
}

message SettingsGetRequest  { string key = 1; }
message SettingsGetResponse { bytes value_json = 1; bool exists = 2; int64 revision = 3; }
message SettingsWatchRequest { string key = 1; }   // 空 key = 整个 namespace
message SettingsChangeEvent  { string key = 1; bytes value_json = 2; int64 revision = 3; }

// ManifestResponse 扩展（追加字段）：
//   bytes settings_schema_json   = 42;  // JSON Schema Draft-07
//   bytes settings_defaults_json = 43;  // 默认值 JSON
```

### 3.3 SDK Go 接口签名

```go
// plugin-sdk/settings.go
type SettingsSchema struct {
    Namespace  string          // 自动 = manifest.Name；admin 页面以此分 tab
    JSONSchema json.RawMessage // Draft-07
    Defaults   json.RawMessage // 与 schema 内 default 等价的 map JSON
}

type SettingsClient interface {
    Get(ctx context.Context, key string) (json.RawMessage, error)
    GetTyped(ctx context.Context, key string, out any) error
    Watch(ctx context.Context, key string) (<-chan SettingsChange, error)
}

type SettingsChange struct {
    Key      string
    Value    json.RawMessage
    Revision int64
}

func RegisterSettingsSchema(s SettingsSchema) error
func (ctx *PluginContext) Settings() SettingsClient
```

### 3.4 SDK 客户端实现要点（关键 30 行）

```go
type settingsClient struct {
    grpc     pb.SettingsExtensionClient
    cache    sync.Map  // key -> *cachedValue
    watchers sync.Map  // key -> []chan SettingsChange
    logger   *slog.Logger
}

type cachedValue struct {
    value     json.RawMessage
    revision  int64
    fetchedAt time.Time
}

func (c *settingsClient) Get(ctx context.Context, key string) (json.RawMessage, error) {
    // 1. 检查本地缓存（TTL 30s，由 Watch 主动失效）
    if v, ok := c.cache.Load(key); ok && time.Since(v.(*cachedValue).fetchedAt) < 30*time.Second {
        return v.(*cachedValue).value, nil
    }
    // 2. 调 host RPC，5s 超时
    rpcCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    resp, err := c.grpc.Get(rpcCtx, &pb.SettingsGetRequest{Key: key})
    if err != nil { return nil, err }
    if !resp.Exists { return nil, ErrSettingNotFound }
    c.cache.Store(key, &cachedValue{value: resp.ValueJson, revision: resp.Revision, fetchedAt: time.Now()})
    return resp.ValueJson, nil
}

// runWatchLoop 在 SDK Init 阶段启动；接收 host 推送 → 更新 cache → fan-out 到订阅者
func (c *settingsClient) runWatchLoop(ctx context.Context) {
    backoff := 1 * time.Second
    for {
        if ctx.Err() != nil { return }
        stream, err := c.grpc.Watch(ctx, &pb.SettingsWatchRequest{Key: ""}) // 全 namespace watch
        if err != nil { c.sleep(ctx, backoff); backoff = min2(backoff*3, 30*time.Second); continue }
        backoff = 1 * time.Second
        for {
            evt, err := stream.Recv()
            if err != nil { break }
            c.cache.Store(evt.Key, &cachedValue{
                value: evt.ValueJson, revision: evt.Revision, fetchedAt: time.Now(),
            })
            c.fanout(SettingsChange{Key: evt.Key, Value: evt.ValueJson, Revision: evt.Revision})
        }
    }
}
```

### 3.5 Host 服务端实现要点

**新增文件**：
- `backend/internal/service/plugin_settings_service.go`（~200 行）：CRUD + ajv schema 校验（用 `github.com/santhosh-tekuri/jsonschema/v5`）+ revision 自增 + LISTEN/NOTIFY fan-out。
- `backend/internal/handler/admin/plugin_settings_handler.go`（~120 行）：`GET /admin/settings/plugins`（列出所有 plugin 的 schema）/ `GET /admin/settings/plugins/:name`（取 schema + 当前值）/ `PUT /admin/settings/plugins/:name`（写 + 校验）。
- `backend/internal/plugin/settings_extension_server.go`（~150 行）：实现 `pb.SettingsExtensionServer`。

**新增 migration**（host 端，非 plugin）：
```sql
-- backend/migrations/130_plugin_settings.sql
CREATE TABLE plugin_settings (
    plugin_name TEXT NOT NULL,
    key         TEXT NOT NULL,
    value_json  JSONB NOT NULL,
    revision    BIGINT NOT NULL DEFAULT 1,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (plugin_name, key)
);
CREATE INDEX idx_plugin_settings_updated_at ON plugin_settings(updated_at);
```

**关键 dispatch**：
```go
// settings_extension_server.go
func (s *SettingsExtensionServer) Get(ctx context.Context, req *pb.SettingsGetRequest) (*pb.SettingsGetResponse, error) {
    pluginName := s.resolver(ctx)
    val, rev, err := s.svc.GetByKey(ctx, pluginName, req.Key)
    if errors.Is(err, sql.ErrNoRows) {
        return &pb.SettingsGetResponse{Exists: false}, nil
    }
    if err != nil { return nil, err }
    return &pb.SettingsGetResponse{ValueJson: val, Revision: rev, Exists: true}, nil
}

func (s *SettingsExtensionServer) Watch(req *pb.SettingsWatchRequest, stream pb.SettingsExtension_WatchServer) error {
    pluginName := s.resolver(stream.Context())
    ch, unsubscribe := s.svc.Subscribe(pluginName, req.Key)
    defer unsubscribe()
    for {
        select {
        case <-stream.Context().Done(): return nil
        case evt := <-ch:
            if err := stream.Send(&pb.SettingsChangeEvent{
                Key: evt.Key, ValueJson: evt.Value, Revision: evt.Revision,
            }); err != nil { return err }
        }
    }
}
```

### 3.6 失败/降级具体策略

| 场景 | 行为 | 数字 |
|------|------|-----|
| schema 校验失败（admin 提交不合法值） | host 返回 422 + `{code: 42201, message: "schema validation failed", details: {field, expected}}` | — |
| plugin Register schema 失败（manifest 内 JSON Schema 不合法） | plugin 启动失败（与 W1 一致） | — |
| Watch stream 断开 | SDK 自动重连 + 全量同步当前值（启动后第一次 Watch 会 emit 全量当前值） | 退避 1s/3s/9s/max 30s |
| plugin 重启时 schema 收紧导致已存值不兼容 | host 不删旧值；plugin Get 时返回原始 JSON，plugin 自己降级（建议 SDK GetTyped 失败时 fallback Defaults） | — |
| RegisterSettingsSchema 超时 | 5s 超时；失败立即 reject，plugin 启动失败 | 5s |

### 3.7 capability 声明 + opt-in

`Capabilities: []string{pluginsdk.CapabilitySettingsExtension}` — host 默认放行。

### 3.8 跨进程边界图

```
plugin                              host
  │                                  │
  │── GetManifest() ─────────────────▶│
  │   (settings_schema_json+defaults) │
  │                                  ├─ PluginSettingsService.UpsertSchema
  │                                  │  schema → 校验 → 存入内存（含 ajv compiled）
  │                                  │  defaults → 写入 plugin_settings（仅缺失 key 写默认）
  │                                  │
  │── Init() ◀───────────────────────│
  │── Settings.Get("enabled") ──────▶│  resolver → "channel-monitor"
  │◀─ {value:"true",revision:1} ─────│
  │                                  │
  │── Settings.Watch("") ───────────▶│  Subscribe pluginName=channel-monitor
  │                                  │  (PG LISTEN "plugin_settings_channel-monitor")
  │                                  │
  │           ─────── admin 在 UI 改 enabled=false ───────
  │                                  │  PUT /admin/settings/plugins/channel-monitor
  │                                  │  ajv validate → ok
  │                                  │  UPDATE plugin_settings SET value_json='false', revision=2
  │                                  │  PG NOTIFY plugin_settings_channel-monitor '{key:"enabled",rev:2}'
  │◀─ Recv() = {key:"enabled",       │
  │     value:"false",revision:2} ◀──│
  │   cache 失效 + fanout(change)     │
```

### 3.9 业界参考（W3）

1. **Backstage `configSchema`** ([backstage.io](https://backstage.io/docs/conf/defining/)) — plugin package.json 内 JSON Schema，host 拼接渲染表单，`x-visibility` 关键字直接借用。
2. **vue-json-schema-form by crickford** ([crickford.github.io/vue-json-schema-form](https://crickford.github.io/vue-json-schema-form/)) — Draft-07 + Vue 3 + 组件无关（D2 选定）。
3. **Sentry plugin Settings API** — 与本设计 `SettingsSchema` 几乎一一对应。

---

## 4. W4 — SafeOutboundHTTPCapability

### 4.1 命名 + 常量

```go
const CapabilitySafeOutboundHTTP = "safe_outbound_http"
```

### 4.2 完整 proto 定义

**无新增 RPC**。在已有 `PluginInitRequest` 上加字段（host → plugin 的 init 阶段下发）：

```proto
message PluginInitRequest {
  // ... 已有字段 1-4 ...
  // 新增：host 在 init 阶段下发的全局出站策略
  OutboundDefaults outbound_defaults = 50;
}

message OutboundDefaults {
  repeated string blocked_cidrs  = 1;   // RFC1918 + 169.254 + 100.64 + admin 自定义
  repeated string allowed_hosts  = 2;   // 全局白名单（admin 配置）
  int32  max_redirects           = 3;   // 默认 3
  int64  timeout_nanos           = 4;   // 默认 30s
  int64  max_body_bytes          = 5;   // 默认 1MiB
}
```

### 4.3 SDK Go 接口签名

```go
// plugin-sdk/outbound.go
type OutboundConfig struct {
    AllowedSchemes    []string
    AllowedPorts      []int
    ExtraBlockedCIDRs []string  // 在 host 下发的 default 上叠加
    AllowedHosts      []string  // plugin 自定义白名单（与 host 全局白名单合并）
    MaxRedirects      int
    MaxBodyBytes      int64
    Timeout           time.Duration
    Proxy             string
}

// NewSafeHTTPClient 返回 SSRF-safe http.Client。
// 实现基于 doyensec/safeurl + 自定义 DialContext（TOCTOU-safe，防 DNS rebinding）。
func NewSafeHTTPClient(cfg OutboundConfig) (*http.Client, error)

// LimitedReadAll 是 ReadAll 的安全包装：超过 MaxBodyBytes 截断 + 返回 ErrBodyTooLarge。
func LimitedReadAll(r io.Reader, max int64) ([]byte, error)

var (
    ErrBlockedTarget    = errors.New("safeurl: target IP in block list")
    ErrTooManyRedirects = errors.New("safeurl: redirect cap reached")
    ErrBodyTooLarge     = errors.New("safeurl: response body exceeds max bytes")
)
```

### 4.4 SDK 客户端实现要点（关键 30 行）

```go
// plugin-sdk/outbound.go
import "github.com/doyensec/safeurl"

func NewSafeHTTPClient(cfg OutboundConfig) (*http.Client, error) {
    if len(cfg.AllowedSchemes) == 0 { cfg.AllowedSchemes = []string{"https"} }
    if cfg.MaxRedirects == 0 { cfg.MaxRedirects = 3 }
    if cfg.Timeout == 0 { cfg.Timeout = 30 * time.Second }
    if cfg.MaxBodyBytes == 0 { cfg.MaxBodyBytes = 1 << 20 }

    builder := safeurl.GetConfigBuilder().
        SetAllowedSchemes(cfg.AllowedSchemes...).
        SetAllowedPorts(cfg.AllowedPorts...).
        SetTimeout(cfg.Timeout)

    // 与 host 下发的 defaults 合并
    defaults := outboundDefaultsFromInit() // SDK 内全局，由 runner 从 PluginInitRequest 填充
    builder = builder.SetBlockedIPsCIDR(append(defaults.BlockedCIDRs, cfg.ExtraBlockedCIDRs...)...)

    if len(cfg.AllowedHosts) > 0 || len(defaults.AllowedHosts) > 0 {
        hosts := append([]string{}, defaults.AllowedHosts...)
        hosts = append(hosts, cfg.AllowedHosts...)
        builder = builder.SetAllowedHosts(hosts...)
    }

    cli := safeurl.Client(builder.Build())
    // safeurl.Client 已封装好；外层 wrap MaxBodyBytes via Transport
    cli.Transport = &limitedBodyTransport{base: cli.Transport, max: cfg.MaxBodyBytes}
    cli.CheckRedirect = func(req *http.Request, via []*http.Request) error {
        if len(via) >= cfg.MaxRedirects { return ErrTooManyRedirects }
        return nil
    }
    return cli, nil
}
```

### 4.5 Host 服务端实现要点

**改动文件**：`backend/internal/plugin/manager.go` `startInstance` 中调 `Init()` 前，从 `setting_service`（已有全局出站设置）拼出 `OutboundDefaults`，塞进 `PluginInitRequest`。

```go
// manager.go startInstance 内 Init() 调用前
var outboundDefaults *pb.OutboundDefaults
if m.settingService != nil {
    outboundDefaults = m.settingService.GetOutboundDefaults(ctxInit)
}
initResp, initErr := lifecycle.Init(ctxInit, &pb.PluginInitRequest{
    SdkAddress:       m.sdkAddr,
    Config:           m.cfg.PluginConfig(inst.Name),
    PluginName:       inst.Name,
    Capabilities:     approvedCaps,
    OutboundDefaults: outboundDefaults,  // 新增
})
```

**新增 host helper**：`backend/internal/service/setting_service.go` 增加 `GetOutboundDefaults(ctx) *pb.OutboundDefaults`，返回管理员配置的全局黑/白名单 + 默认 timeout。

### 4.6 失败/降级具体策略

| 场景 | 行为 | 数字 |
|------|------|-----|
| 解析后 IP 命中黑名单 | `ErrBlockedTarget` | — |
| DNS 解析失败 | 标准 `*url.Error` | timeout 30s |
| redirect > MaxRedirects | `ErrTooManyRedirects` | 默认 3 |
| 响应 body > MaxBodyBytes | `io.LimitReader` 截断 + warn log + `ErrBodyTooLarge` | 默认 1MiB |
| DNS rebinding 攻击（TTL=0 第二次解析变内网 IP） | safeurl 的 `DialContext` Control hook 在每次拨号都重新校验 IP | — |
| 全局黑/白名单热更新 | 通过 W3 SettingsExtension Watch 推回 plugin；SDK 重建 outboundDefaults，下次 NewSafeHTTPClient 生效（已创建的 client 不更新，符合"配置变更不影响进行中请求"语义） | — |

### 4.7 capability 声明 + opt-in

`Capabilities: []string{pluginsdk.CapabilitySafeOutboundHTTP}` — 默认放行。capability 字符串保留供 V6 收紧（如限制只能调白名单内 host）。

### 4.8 跨进程边界图

```
plugin                              host
  │                                  │
  │── Init() ◀──────────────────────│
  │   PluginInitRequest{             │
  │     outbound_defaults:{          │
  │       blocked_cidrs:[10/8,...],  │
  │       allowed_hosts:[],          │
  │       max_redirects:3,           │
  │       timeout:30s}}              │
  │                                  │
  │  SDK 把 defaults 缓存到全局        │
  │  outboundDefaults 变量            │
  │                                  │
  │  NewSafeHTTPClient(cfg)          │
  │  → 合并 defaults + cfg            │
  │  → safeurl.Client (内部封装        │
  │     DialContext，每次拨号 re-check)│
  │                                  │
  │── client.Get("https://api.foo.com")
  │     DNS resolve → 1.2.3.4        │
  │     check 1.2.3.4 not in blocked │
  │     dial → ok                    │
  │     read body (limit 1MiB)       │
```

### 4.9 业界参考（W4）

1. **doyensec/safeurl** ([github.com/doyensec/safeurl](https://github.com/doyensec/safeurl)) — DialContext Control hook 做 TOCTOU-safe 校验。
2. **CVE-2026-41488 LangChain** ([advisories.gitlab.com](https://advisories.gitlab.com/pypi/langchain-openai/GHSA-r7w7-9xr2-qq2r/)) — 反面教材：`_url_to_size()` 验证后再单独 DNS 解析，被 DNS rebinding 绕过。
3. **CVE-2026-41055 AVideo** ([vulnerability.circl.lu](https://vulnerability.circl.lu/vuln/cve-2026-41055)) — 反面教材：`isSSRFSafeURL()` 与 `get_headers()` 两次独立 DNS 解析，仍被 TTL=0 攻击拿下。

---

## 5. W5 — SecretEncryptionCapability

### 5.1 命名 + 常量

```go
const CapabilitySecretEncryption = "secret_encryption"
```

显式 capability — 加密能力涉及密钥派生，host 必须知道哪些 plugin 持有密文，便于审计。

### 5.2 完整 proto 定义

```proto
service SecretEncryption {
  rpc Encrypt(EncryptRequest) returns (EncryptResponse);
  rpc Decrypt(DecryptRequest) returns (DecryptResponse);
}
message EncryptRequest  { bytes plaintext  = 1; }
message EncryptResponse { bytes ciphertext = 1; }   // = nonce(12) || ciphertext || tag(16)
message DecryptRequest  { bytes ciphertext = 1; }
message DecryptResponse { bytes plaintext  = 1; }
```

### 5.3 SDK Go 接口签名

```go
// plugin-sdk/secrets.go
type SecretEncryptor interface {
    Encrypt(ctx context.Context, plaintext []byte) ([]byte, error)
    Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error)
}

func (ctx *PluginContext) Secrets() SecretEncryptor

var (
    ErrSecretTooLarge = errors.New("plugin-sdk: secret too large (>64KiB)")
    ErrSecretInvalid  = errors.New("plugin-sdk: secret decrypt failed (corrupted or cross-plugin)")
)

const MaxSecretBytes = 64 * 1024  // 64 KiB
```

### 5.4 SDK 客户端实现要点

```go
// plugin-sdk/secrets.go
type secretsClient struct {
    grpc pb.SecretEncryptionClient
}

func (s *secretsClient) Encrypt(ctx context.Context, plaintext []byte) ([]byte, error) {
    if len(plaintext) > MaxSecretBytes { return nil, ErrSecretTooLarge }
    rpcCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    resp, err := s.grpc.Encrypt(rpcCtx, &pb.EncryptRequest{Plaintext: plaintext})
    if err != nil { return nil, err }
    return resp.Ciphertext, nil
}

func (s *secretsClient) Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error) {
    if len(ciphertext) > MaxSecretBytes+128 { return nil, ErrSecretTooLarge }
    rpcCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    resp, err := s.grpc.Decrypt(rpcCtx, &pb.DecryptRequest{Ciphertext: ciphertext})
    if err != nil {
        if status.Code(err) == codes.InvalidArgument { return nil, ErrSecretInvalid }
        return nil, err
    }
    return resp.Plaintext, nil
}
```

### 5.5 Host 服务端实现要点

**新增文件**：`backend/internal/plugin/secret_encryption_server.go`（~150 行）。

```go
package plugin

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "crypto/sha256"
    "encoding/hex"
    "io"
    "sync"

    "golang.org/x/crypto/hkdf"
)

type SecretEncryptionServer struct {
    pb.UnimplementedSecretEncryptionServer
    masterKey    []byte                  // host 启动时校验：必须 32 bytes
    perPluginKey sync.Map                // pluginName -> []byte (32 bytes)
    resolver     func(ctx context.Context) string
    logger       *zap.Logger
}

func NewSecretEncryptionServer(masterKeyHex string, resolver func(ctx context.Context) string, logger *zap.Logger) (*SecretEncryptionServer, error) {
    key, err := hex.DecodeString(masterKeyHex)
    if err != nil { return nil, fmt.Errorf("invalid master key hex: %w", err) }
    if len(key) != 32 { return nil, fmt.Errorf("master key must be 32 bytes, got %d", len(key)) }
    return &SecretEncryptionServer{masterKey: key, resolver: resolver, logger: logger}, nil
}

func (s *SecretEncryptionServer) deriveKey(pluginName string) ([]byte, error) {
    if v, ok := s.perPluginKey.Load(pluginName); ok { return v.([]byte), nil }
    info := []byte("sub2api-plugin-secret-v1|" + pluginName)
    h := hkdf.New(sha256.New, s.masterKey, []byte(pluginName), info)
    derived := make([]byte, 32)
    if _, err := io.ReadFull(h, derived); err != nil { return nil, err }
    s.perPluginKey.Store(pluginName, derived)
    return derived, nil
}

func (s *SecretEncryptionServer) Encrypt(ctx context.Context, req *pb.EncryptRequest) (*pb.EncryptResponse, error) {
    pluginName := s.resolver(ctx)
    if pluginName == "" { return nil, status.Error(codes.Unauthenticated, "missing caller identity") }
    if len(req.Plaintext) > 64*1024 { return nil, status.Error(codes.ResourceExhausted, "plaintext too large") }
    key, err := s.deriveKey(pluginName)
    if err != nil { return nil, err }
    block, _ := aes.NewCipher(key)
    aead, _ := cipher.NewGCM(block)
    nonce := make([]byte, 12)
    if _, err := rand.Read(nonce); err != nil { return nil, err }
    ct := aead.Seal(nil, nonce, req.Plaintext, []byte(pluginName)) // AAD = pluginName，再加一道墙
    return &pb.EncryptResponse{Ciphertext: append(nonce, ct...)}, nil
}

func (s *SecretEncryptionServer) Decrypt(ctx context.Context, req *pb.DecryptRequest) (*pb.DecryptResponse, error) {
    pluginName := s.resolver(ctx)
    if len(req.Ciphertext) < 12+16 { return nil, status.Error(codes.InvalidArgument, "ciphertext too short") }
    key, err := s.deriveKey(pluginName)
    if err != nil { return nil, err }
    block, _ := aes.NewCipher(key)
    aead, _ := cipher.NewGCM(block)
    nonce, ct := req.Ciphertext[:12], req.Ciphertext[12:]
    pt, err := aead.Open(nil, nonce, ct, []byte(pluginName))
    if err != nil {
        // 注意：不能记 ciphertext / plaintext，只记 plugin + length
        s.logger.Warn("plugin secret decrypt failed",
            zap.String("plugin", pluginName),
            zap.Int("ciphertext_len", len(req.Ciphertext)),
        )
        return nil, status.Error(codes.InvalidArgument, "decrypt failed")
    }
    return &pb.DecryptResponse{Plaintext: pt}, nil
}
```

### 5.6 失败/降级具体策略

| 场景 | 行为 | 数字 |
|------|------|-----|
| host 启动 masterKey 不是 32 bytes | host fail-fast，不启动 | — |
| plaintext > 64KiB | `ErrSecretTooLarge`，gRPC `ResourceExhausted` | 64 KiB |
| Decrypt 校验失败（密文损坏 / 跨 plugin 篡改） | `ErrSecretInvalid`，host audit log（不记 ciphertext / plaintext，仅 plugin name + length） | — |
| RPC 超时 | 5s 超时；网络抖动属于罕见错误，调用方用 `errors.Is(err, ctx.DeadlineExceeded)` 判断 | 5s |
| key derive 失败（HKDF 不会失败，除非 master key 0 长度，已 fail-fast 拦截） | — | — |
| key 轮换 | V6+ 议题；当前不支持，文档明示 | — |

### 5.7 capability 声明 + opt-in

`Capabilities: []string{pluginsdk.CapabilitySecretEncryption}` — host approveCapabilities 默认放行。

### 5.8 跨进程边界图

```
plugin                              host
  │                                  │
  │── Encrypt(plaintext) ───────────▶│  resolver → pluginName="channel-monitor"
  │                                  │  deriveKey("channel-monitor") = HKDF(master, salt=name, info=...)
  │                                  │  AES-GCM Seal(nonce, plaintext, AAD=pluginName)
  │◀─ ciphertext = nonce||ct||tag ───│
  │                                  │
  │  存到 plugin 自己的 monitor 表       │
  │  encrypted_api_key 字段            │
  │                                  │
  │             ─────── 几小时后 ───────
  │── Decrypt(ciphertext) ──────────▶│
  │                                  │  deriveKey("channel-monitor") (cache hit)
  │                                  │  Open(nonce, ct, AAD=pluginName) → plaintext
  │◀─ plaintext ─────────────────────│
  │                                  │
  │  ─── plugin "billing" 偷到 ciphertext 想 Decrypt ───
  │                                  │  resolver → "billing"
  │                                  │  deriveKey("billing") ≠ deriveKey("channel-monitor")
  │                                  │  Open(...) → AAD mismatch → 失败
  │◀─ ErrSecretInvalid ──────────────│  audit log: cross-plugin decrypt attempted
```

### 5.9 业界参考（W5）

1. **HashiCorp Vault Transit Engine** ([developer.hashicorp.com/vault/docs/secrets/transit](https://developer.hashicorp.com/vault/docs/secrets/transit)) — "key 管理 vs plaintext 暴露分离"语义。
2. **AWS Hierarchical Keyring + HKDF** ([AWS docs](https://docs.aws.amazon.com/database-encryption-sdk/latest/devguide/use-hierarchical-keyring.html)) — per-tenant HKDF 派生模型。
3. **RFC 9709 HKDF-SHA256 for CMS** ([datatracker.ietf.org/doc/rfc9709](https://datatracker.ietf.org/doc/rfc9709/)) — info 参数绑定 algorithm + tenant 防算法替换攻击。

---

## 6. W6 / W8 — 迁移代码示意（before / after）

### 6.1 W6 — Channel Monitor 后端关键迁移点

#### 6.1.1 SSRF guard（W4 替换）

**Before** (`channel_monitor_checker.go` + `channel_monitor_ssrf.go` 共 ~595 行)：
```go
// service/channel_monitor_ssrf.go (152 行)
func safeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
    host, _, _ := net.SplitHostPort(addr)
    ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
    if err != nil { return nil, err }
    for _, ip := range ips {
        if isPrivate(ip.IP) || isLoopback(ip.IP) || isMetadata(ip.IP) {
            return nil, fmt.Errorf("blocked IP: %s", ip)
        }
    }
    return (&net.Dialer{Timeout: 10*time.Second}).DialContext(ctx, network, addr)
}
// + 私网/loopback/metadata IP 黑名单 + DNS rebinding 关心 ...
```

**After** (`plugins/channel-monitor/internal/checker.go`)：
```go
// 用 SDK SafeOutboundHTTP，3 行替代 152 行
client, err := sdk.NewSafeHTTPClient(pluginsdk.OutboundConfig{
    AllowedSchemes: []string{"https"},
    MaxBodyBytes:   2 * 1024 * 1024, // 检测响应可能稍大
    Timeout:        30 * time.Second,
})
```

#### 6.1.2 Runner（W2 替换）

**Before** (`channel_monitor_runner.go` 291 行)：
```go
type ChannelMonitorRunner struct {
    monitors    map[int64]*time.Ticker
    workerPool  *pond.WorkerPool
    inFlight    sync.Map
    leaderLock  *OpsCleanupService
}
func (r *ChannelMonitorRunner) Start(ctx context.Context) {
    for _, m := range r.repo.ListEnabled(ctx) {
        ticker := time.NewTicker(m.Interval)
        go r.tickerLoop(ctx, m.ID, ticker)
    }
}
// + RunDailyMaintenance + heartbeat ...
```

**After** (`plugins/channel-monitor/internal/jobs.go`)：
```go
// 删 291 行。Job 注册 < 20 行
sdk.Jobs().Register(pluginsdk.JobSpec{
    Name:        "monitor.run",
    Trigger:     pluginsdk.JobTrigger{Kind: pluginsdk.TriggerInterval, Interval: 60 * time.Second},
    Concurrency: 5,
    Timeout:     2 * time.Minute,
}, runOneTick)

sdk.Jobs().Register(pluginsdk.JobSpec{
    Name:       "monitor.daily-rollup",
    Trigger:    pluginsdk.JobTrigger{Kind: pluginsdk.TriggerCron, CronSpec: "0 2 * * *"},
    LeaderOnly: true,
    Timeout:    10 * time.Minute,
}, runDailyRollup)

func runOneTick(ctx context.Context, _ string) error {
    // 旧 ChannelMonitorRunner 内的 fetch + check 逻辑搬过来，去掉 ticker / pool / leader lock
    return svc.RunOneTickAll(ctx)
}
```

#### 6.1.3 api_key 加密（W5 替换）

**Before** (`service/channel_monitor_service.go`)：
```go
import "github.com/Wei-Shaw/sub2api/internal/service" // 取 SecretEncryptor
// ...
encrypted, err := s.encryptor.Encrypt(ctx, []byte(apiKey))  // host 内 SecretEncryptor
```

**After** (`plugins/channel-monitor/internal/service.go`)：
```go
// SDK 透出 encryptor，plugin 进程拿不到 master key
encrypted, err := sdk.Secrets().Encrypt(ctx, []byte(apiKey))
```

#### 6.1.4 Settings + feature flag（W3 替换）

**Before** (`backend/internal/service/setting_service.go`)：
```go
func (s *SettingService) GetChannelMonitorRuntime(ctx context.Context) ChannelMonitorRuntime {
    return ChannelMonitorRuntime{
        Enabled: s.cfg.ChannelMonitor.Enabled,
        DefaultIntervalSec: s.cfg.ChannelMonitor.DefaultIntervalSec,
        // ... 12 个字段，每加一个都要改 setting_service.go + dto + 前端
    }
}
```

**After** (`plugins/channel-monitor/manifest.go` + `internal/runtime.go`)：
```go
// manifest.go 内嵌 schema
//go:embed settings_schema.json
var settingsSchemaJSON []byte
//go:embed settings_defaults.json
var settingsDefaultsJSON []byte

func (p *Plugin) Manifest() *pluginsdk.Manifest {
    return &pluginsdk.Manifest{
        // ...
        SettingsSchema:   settingsSchemaJSON,
        SettingsDefaults: settingsDefaultsJSON,
    }
}

// runtime.go 取值
type runtime struct {
    Enabled            bool   `json:"enabled"`
    DefaultIntervalSec int    `json:"defaultIntervalSec"`
    TemplateMaxBodyKB  int    `json:"templateMaxBodyKB"`
    DailyRollupHourUTC int    `json:"dailyRollupHourUTC"`
}
func currentRuntime(ctx context.Context) (runtime, error) {
    var rt runtime
    if err := sdk.Settings().GetTyped(ctx, "", &rt); err != nil { return rt, err }
    return rt, nil
}
```

### 6.2 W8 — Available Channels 后端的 "snapshot copy" 模式

> **D1 决策**：W8 走 SQLProxy，但 host 的 `Channel.SupportedModels` 纯逻辑（170 行）通过"vendor copy"搬到 plugin。

**Before** (`backend/internal/service/channel.go::SupportedModels` 170 行)：
```go
func (c *Channel) SupportedModels(pricings []ModelPricing) []SupportedModel {
    // mapping ∪ pricing 并联
    // wildcard 展开（"openai/*" → 列出所有 openai/ 前缀的 model）
    // billing mode 判断
    // ... 170 行业务逻辑
}
```

**After** (`plugins/available-channels/internal/domain/channel_view.go`)：
```go
// VENDORED FROM commit 09fd83ab:backend/internal/service/channel.go
// at 2026-04-26. Sync requires: regenerate by running tools/sync-channel-domain.sh
// when host channel logic evolves. See V5-DESIGN.md §6.2.
//
// 长期方向：V6 HostServiceProxyCapability 取代此 vendor copy。

type ChannelView struct { /* 同 host 的 Channel struct */ }

func (c *ChannelView) SupportedModels(pricings []ModelPricing) []SupportedModel {
    // 完整复制逻辑，仅替换 ent 调用为 SQLProxy 查询
}
```

**Listener handler**:
```go
// plugins/available-channels/internal/handler.go
func listAvailable(ctx *gin.Context) {
    rows, _ := sdk.SQL().Query(ctx,
        "SELECT id, name, platform, channel_mappings_json, ... FROM channels WHERE status=$1", "active")
    channels := parseChannels(rows)

    pricingRows, _ := sdk.SQL().Query(ctx, "SELECT * FROM model_pricings")
    pricings := parsePricings(pricingRows)

    out := make([]SupportedChannelView, 0, len(channels))
    for _, c := range channels {
        models := c.SupportedModels(pricings) // ← vendored
        out = append(out, SupportedChannelView{Channel: c, Models: models})
    }
    ctx.JSON(200, gin.H{"data": out})
}
```

> **关键约束**：vendor copy 的文件**头注释必须**包含 source commit + 同步脚本路径，否则 V5 验收 fail。Implementer 任务图见 §7 W8.2。

---

## 7. 任务图细化（接 Curator W1-W10，每 W 拆 2-5 Step）

> 每 Step 含：**改动文件**、**输入依赖**、**输出 commit**、**验证命令**。Implementer sub-agent 一次只接一个 Step。

### W1 — MigrationRunnerCapability（4 Steps）

| Step | 改动文件 | 输入依赖 | 输出 commit | 验证命令 |
|------|---------|----------|------------|---------|
| W1.1 | `plugin-sdk/proto/plugin.proto` 加 `GetMigration` RPC + `MigrationDecl`；`plugin-sdk/proto/buf.gen.yaml` 重跑 | 无 | `feat(plugin-sdk): add MigrationProxy RPC + MigrationDecl` | `cd plugin-sdk && buf generate && go build ./...` |
| W1.2 | `plugin-sdk/migration.go` 新建（`embedProvider` + `RegisterMigrationProvider`）；`plugin-sdk/runner.go` 新增 `lifecycleServer.GetMigration` handler | W1.1 proto | `feat(plugin-sdk): MigrationProvider + GetMigration handler` | `cd plugin-sdk && go test ./...` |
| W1.3 | `backend/internal/plugin/manager.go` 700-705 行替换为 `fetchPluginMigrations` + `RunPluginMigrations` 调用；新增 `fetchPluginMigrations` helper | W1.1 proto + 现有 `migrations.go` | `feat(plugin): fetch and apply plugin migrations from SDK` | `cd backend && go build ./... && go test ./internal/plugin/...` |
| W1.4 | `plugin-sdk/manifest.go` 加 `CapabilityMigrationRunner` 常量；`backend/internal/plugin/grpc_server.go` allow-list 加入 | W1.1-W1.3 | `feat(plugin-sdk): declare migration_runner capability` | `grep CapabilityMigrationRunner plugin-sdk/ backend/internal/plugin/` |

### W2 — JobSchedulerCapability（5 Steps）

| Step | 改动文件 | 输入依赖 | 输出 commit | 验证命令 |
|------|---------|----------|------------|---------|
| W2.1 | `plugin-sdk/proto/sdk.proto` 加 `JobScheduler` service + 5 个 message | 无 | `feat(plugin-sdk): add JobScheduler proto` | `buf generate` |
| W2.2 | `plugin-sdk/jobs.go` 新建（jobsClient + runJobLoop + dispatch + sem）；扩展 `PluginContext.Jobs()` | W2.1 | `feat(plugin-sdk): JobsClient with reconnect + concurrency limit` | `go test ./plugin-sdk/...` |
| W2.3 | `backend/internal/plugin/job_scheduler_server.go` 新建（PluginScheduler + Subscribe + cron 集成）；引入 `github.com/robfig/cron/v3 v3.0.1` 到 go.mod | W2.1 + 现有 `OpsCleanupService.tryAcquireLeaderLock` | `feat(plugin): JobScheduler server with leader lock + cron` | `cd backend && go test ./internal/plugin/...` |
| W2.4 | `backend/internal/handler/admin/plugin_jobs_handler.go` 新建：`POST /api/v1/admin/plugins/:name/jobs/:job/trigger` → `ManualFire`；`POST /api/v1/admin/plugins/:name/jobs/:job/history?limit=20` 查 history；新增 `backend/migrations/131_plugin_job_history.sql` | W2.3 | `feat(plugin): admin run-now + job history endpoint` | `curl -X POST /admin/plugins/x/jobs/y/trigger -H "x-api-key: $KEY"` |
| W2.5 | `plugin-sdk/manifest.go` 加 `CapabilityJobScheduler`；`backend/internal/plugin/grpc_server.go` allow-list；`backend/internal/plugin/wire.go` 注入 JobSchedulerServer | W2.1-W2.4 | `feat(plugin-sdk): wire JobScheduler into SDK gRPC` | `cd backend && go build ./... && cd plugin-sdk && go build ./...` |

### W3 — SettingsExtensionCapability（4 Steps）

| Step | 改动文件 | 输入依赖 | 输出 commit | 验证命令 |
|------|---------|----------|------------|---------|
| W3.1 | `plugin-sdk/proto/sdk.proto` 加 `SettingsExtension` service；`plugin-sdk/proto/plugin.proto` ManifestResponse 加 `settings_schema_json` + `settings_defaults_json` | 无 | `feat(plugin-sdk): SettingsExtension proto` | `buf generate` |
| W3.2 | `plugin-sdk/settings.go` 新建（settingsClient + cache + Watch loop + fanout）；`PluginContext.Settings()` | W3.1 | `feat(plugin-sdk): SettingsClient with cache + watch` | `go test ./plugin-sdk/...` |
| W3.3 | `backend/migrations/130_plugin_settings.sql`；`backend/internal/service/plugin_settings_service.go` 新建（CRUD + ajv schema 校验 + LISTEN/NOTIFY）；`backend/internal/plugin/settings_extension_server.go` 新建 | W3.1 + `github.com/santhosh-tekuri/jsonschema/v5` | `feat(plugin): plugin settings storage + watch fanout` | `psql -c "\\d plugin_settings"` |
| W3.4 | `backend/internal/handler/admin/plugin_settings_handler.go`（GET/PUT）；前端 `frontend/src/views/admin/SettingsView.vue` 新增"Plugins" tab 用 `vue-json-schema-form` (crickford 版) 渲染；如 PoC fail 则用手写 `<DynamicSettingsForm/>` | W3.3 + Designer D2 决策 | `feat(plugin): admin settings UI + dynamic schema form` | 浏览器访问 `/admin/settings` 看到 Plugins tab |

### W4 — SafeOutboundHTTPCapability（3 Steps）

| Step | 改动文件 | 输入依赖 | 输出 commit | 验证命令 |
|------|---------|----------|------------|---------|
| W4.1 | `plugin-sdk/proto/plugin.proto` PluginInitRequest 加 `OutboundDefaults`；`plugin-sdk/outbound.go` 新建（NewSafeHTTPClient + LimitedReadAll）；引入 `github.com/doyensec/safeurl` | 无 | `feat(plugin-sdk): SafeOutboundHTTP client` | `go build ./plugin-sdk/...` |
| W4.2 | `backend/internal/service/setting_service.go` 加 `GetOutboundDefaults`；`backend/internal/plugin/manager.go` Init 调用时塞入 InitRequest | W4.1 | `feat(plugin): host outbound defaults broadcast` | `cd backend && go build ./...` |
| W4.3 | `plugin-sdk/outbound_test.go` 单测：调 `http://169.254.169.254/`、`http://10.0.0.1/`、DNS rebinding mock case 全部返回 `ErrBlockedTarget` | W4.1 | `test(plugin-sdk): SSRF guard cases` | `go test -run TestSafeOutbound ./plugin-sdk/` |

### W5 — SecretEncryptionCapability（3 Steps）

| Step | 改动文件 | 输入依赖 | 输出 commit | 验证命令 |
|------|---------|----------|------------|---------|
| W5.1 | `plugin-sdk/proto/sdk.proto` 加 `SecretEncryption` service；`plugin-sdk/secrets.go` 新建（secretsClient + Encrypt/Decrypt with timeout） | 无 | `feat(plugin-sdk): SecretEncryption client` | `buf generate && go build ./...` |
| W5.2 | `backend/internal/plugin/secret_encryption_server.go` 新建（HKDF derive + AES-GCM + AAD=pluginName + perPluginKey cache + audit log）；`backend/internal/plugin/wire.go` 注入；引入 `golang.org/x/crypto/hkdf`（应已在 go.sum） | W5.1 + host `.env ENCRYPTION_KEY` | `feat(plugin): SecretEncryption server with HKDF + per-plugin AAD` | 单测：plugin A 加密 → plugin B Decrypt 返回 ErrSecretInvalid |
| W5.3 | `plugin-sdk/manifest.go` 加 `CapabilitySecretEncryption`；`backend/internal/plugin/grpc_server.go` allow-list | W5.1-W5.2 | `feat(plugin-sdk): wire SecretEncryption capability` | `grep CapabilitySecretEncryption plugin-sdk/ backend/internal/plugin/` |

### W6 — Channel Monitor 后端迁移（5 Steps）

| Step | 改动文件 | 输入依赖 | 输出 commit | 验证命令 |
|------|---------|----------|------------|---------|
| W6.1 | `plugins/channel-monitor/` 目录结构搭建 + `manifest.go` + 4 个 SQL 文件从 `git show 09fd83ab:backend/migrations/12{5..8}_*.sql` 拷入 `plugins/channel-monitor/migrations/00{1..4}_*.sql` + `embed.FS` | W1（migration runner） | `feat(channel-monitor): bootstrap plugin layout + migrations` | `psql 看到 4 张表` |
| W6.2 | 拷贝 `git show 09fd83ab:backend/internal/service/channel_monitor_*.go` 到 `plugins/channel-monitor/internal/`；改 import 从 host service 到 SDK；删除 SSRF guard（W4 替换）；删除 SecretEncryptor 依赖（W5 替换） | W6.1 + W4 + W5 | `feat(channel-monitor): port checker/aggregator/template service to plugin` | `cd plugins/channel-monitor && go build ./...` |
| W6.3 | 拷贝 `runner.go` → 改为 `jobs.go`，删除 ticker/pool/leader 自管，改用 `sdk.Jobs().Register` 两个 spec | W6.2 + W2 | `feat(channel-monitor): replace runner with JobScheduler` | `Go build + admin Run-now 按钮可触发` |
| W6.4 | 拷贝 `repository/channel_monitor_*_repo.go` → 改 ent 调用为 SDK SQLProxy；handler 拷贝到 `plugins/channel-monitor/internal/handler/`；改 plugin gateway 路由 manifest | W6.2 | `feat(channel-monitor): repository on SQLProxy + handlers` | `curl /admin/plugins/channel-monitor/monitors` |
| W6.5 | 把 host `setting_service.GetChannelMonitorRuntime` 删除；plugin 内 `settings_schema.json` + `settings_defaults.json` 写好 4 个字段；`runtime.go` 用 `sdk.Settings().GetTyped` 读 | W6.2 + W3 | `feat(channel-monitor): settings via SettingsExtension` | admin /settings 看到 Channel Monitor tab |

### W7 — Channel Monitor 前端迁移（3 Steps）

| Step | 改动文件 | 输入依赖 | 输出 commit | 验证命令 |
|------|---------|----------|------------|---------|
| W7.1 | 从 09fd83ab 拷贝所有 vue/ts 文件到 `plugins/channel-monitor/frontend/`；改 manifest 注册菜单 + 路由 | W6（API 形状契约） | `feat(channel-monitor): port frontend views/components` | `pnpm build` plugin 入口 |
| W7.2 | i18n：`sdk.i18n.registerNamespace('monitor', ...)`；feature_flag 改用 W3 settings；删除 host 端的 `monitor.*`/`monitorCommon.*` namespace | W7.1 + W3 | `feat(channel-monitor): plugin-side i18n + feature flag` | `/admin/monitor` 中英切换正常 |
| W7.3 | 用户页 `/channel-status` 注册到 SectionUser；MonitorDetailDialog 等用户组件 | W7.1 | `feat(channel-monitor): user-facing channel status view` | 用户登录访问 `/channel-status` |

### W8 — Available Channels 后端迁移（3 Steps）

| Step | 改动文件 | 输入依赖 | 输出 commit | 验证命令 |
|------|---------|----------|------------|---------|
| W8.1 | `plugins/available-channels/` 目录结构 + `manifest.go` + `settings_schema.json`（仅 `enabled` 字段）；feature flag 用 W3 | W3 | `feat(available-channels): bootstrap + feature flag via SettingsExtension` | admin /settings 看到 Available Channels tab |
| W8.2 | **vendor copy** `git show 09fd83ab:backend/internal/service/channel.go` 中 `SupportedModels` 函数及周边 struct 到 `plugins/available-channels/internal/domain/channel_view.go`，文件头加 `VENDORED FROM commit 09fd83ab` 注释 + 同步脚本 stub `tools/sync-channel-domain.sh` | 09fd83ab 的 channel.go | `feat(available-channels): vendor channel domain logic` | `head -10 plugins/available-channels/internal/domain/channel_view.go` 看到 VENDORED 注释 |
| W8.3 | `plugins/available-channels/internal/handler.go`：`GET /api/v1/channels/available`，走 SDK SQLProxy 拉 channels + pricings + groups，调 vendored `SupportedModels`；用户身份过滤通过 V4 `X-Plugin-User-*` header | W8.1 + W8.2 | `feat(available-channels): user-facing list endpoint` | `curl -H "Cookie: ..." /api/v1/channels/available` |

### W9 — Available Channels 前端迁移（2 Steps）

| Step | 改动文件 | 输入依赖 | 输出 commit | 验证命令 |
|------|---------|----------|------------|---------|
| W9.1 | 从 09fd83ab 拷 `AvailableChannelsView.vue` + `AvailableChannelsTable.vue` + `PricingRow.vue` + `SupportedModelChip.vue` 到 `plugins/available-channels/frontend/`；i18n 注册 `availableChannels.*` | W8 | `feat(available-channels): port frontend components` | `pnpm build plugin entry` |
| W9.2 | manifest 注册路由 + 菜单（Section=user）；feature_flag 走 W3 settings | W9.1 + W3 | `feat(available-channels): menu/route via plugin manifest` | 用户访问 `/available-channels` 看到卡片网格 |

### W10 — E2E 验收（2 Steps）

| Step | 改动文件 | 输入依赖 | 输出 commit | 验证命令 |
|------|---------|----------|------------|---------|
| W10.1 | `tests/e2e/v5_capabilities_test.sh`：5 项能力 smoke test（migration applied / job triggered / setting watch fired / SSRF blocked / secret cross-plugin denied） | W1-W5 | `test(e2e): V5 capabilities smoke test` | bash 脚本全绿 |
| W10.2 | beta 部署 + 按 V5-CURATE §5 验收清单 10 项逐项打勾；写 `docs/plugin-architecture/V5-VERIFY.md` | W6-W9 + W10.1 | `docs(plugin-architecture): V5 verification report` | beta `/admin/monitor` + `/available-channels` 都能用 |

> 总计 **34 Steps**（W1=4 + W2=5 + W3=4 + W4=3 + W5=3 + W6=5 + W7=3 + W8=3 + W9=2 + W10=2）。

---

## 8. 业界参考索引（5 能力 + 整体架构）

| 能力/主题 | URL | 说明 |
|-----------|-----|------|
| W1 MigrationRunner — pg-safe-migrate | https://dev.to/defnotwig/pg-safe-migrate-stop-shipping-unsafe-postgres-migrations-1m5n | advisory lock + checksum + non_transactional 检测 |
| W1 MigrationRunner — Atlas Cloud | https://atlasgo.io | host-managed migration vs plugin-declared |
| W1 MigrationRunner — HashiCorp Nomad task driver plugins | https://github.com/hashicorp/go-plugin | Capabilities RPC 模型 |
| W2 JobScheduler — River | https://github.com/riverqueue/river | work fn 注册 + cron/interval 触发分离 |
| W2 JobScheduler — Asynq | https://github.com/hibiken/asynq | leader 单例 + worker 多实例 |
| W2 JobScheduler — robfig/cron/v3 | https://github.com/robfig/cron | host 端 cron 表达式解析（D3 选定） |
| W3 SettingsExtension — Backstage configSchema | https://backstage.io/docs/conf/defining/ | plugin → host 拼接渲染表单 |
| W3 SettingsExtension — vue-json-schema-form (crickford) | https://crickford.github.io/vue-json-schema-form/ | Draft-07 + Vue 3 + 组件无关（D2 选定） |
| W3 SettingsExtension — Sentry plugin Settings API | https://docs.sentry.io/product/integrations/integration-platform/ | 同质语义 |
| W4 SafeOutboundHTTP — doyensec/safeurl | https://github.com/doyensec/safeurl | TOCTOU-safe DialContext |
| W4 SafeOutboundHTTP — CVE-2026-41488 LangChain | https://advisories.gitlab.com/pypi/langchain-openai/GHSA-r7w7-9xr2-qq2r/ | 反面教材：DNS rebinding 绕过 |
| W4 SafeOutboundHTTP — CVE-2026-41055 AVideo | https://vulnerability.circl.lu/vuln/cve-2026-41055 | 反面教材：incomplete SSRF fix |
| W5 SecretEncryption — Vault Transit | https://developer.hashicorp.com/vault/docs/secrets/transit | key 管理 vs plaintext 暴露分离 |
| W5 SecretEncryption — AWS Hierarchical Keyring | https://docs.aws.amazon.com/database-encryption-sdk/latest/devguide/use-hierarchical-keyring.html | per-tenant HKDF 派生 |
| W5 SecretEncryption — RFC 9709 | https://datatracker.ietf.org/doc/rfc9709/ | HKDF info 绑定 algorithm + tenant |
| 整体架构 — HashiCorp go-plugin GRPCBroker | https://github.com/hashicorp/go-plugin/blob/main/docs/extensive-go-plugin-tutorial.md | 双向通信 + host service proxy 思路（V6 储备） |

> 共 **16 个**业界参考 URL（每能力 2-4 个）。

---

## 9. 命名 plugin-agnostic 自检

```bash
# 5 个 capability 字符串、proto service 名、SDK Go 类型 / 函数名全部不含 channel
grep -E "(Channel|Monitor|Available|Subscription|Order)" docs/plugin-architecture/V5-DESIGN.md \
  | grep -v "^[│├└]" \
  | grep -v "Channel Monitor" \
  | grep -v "Available Channels" \
  | grep -v "channel_monitor" \
  | grep -v "channel.go" \
  | grep -v "available_channel"
# 期望：0 命中（除迁移示范段引用旧文件名）
```

5 个 capability 名最终敲定：

| 序号 | capability 字符串 | proto service 名 | SDK Go 入口 |
|-----|------------------|------------------|------------|
| W1 | `migration_runner` | `PluginLifecycle.GetMigration` | `RegisterMigrationProvider` |
| W2 | `job_scheduler` | `JobScheduler` | `PluginContext.Jobs()` |
| W3 | `settings_extension` | `SettingsExtension` | `PluginContext.Settings()` |
| W4 | `safe_outbound_http` | （无 RPC，纯 SDK + InitRequest 字段） | `NewSafeHTTPClient` |
| W5 | `secret_encryption` | `SecretEncryption` | `PluginContext.Secrets()` |

---

## 10. V5 Out of Scope → V6 储备

| V6 第 N 项 | 来源 | 触发条件 |
|-----------|------|---------|
| 1 | **HostServiceProxyCapability** — plugin 通过 gRPC 调 host 注册的 service 方法 | W8 vendor copy 出现第二个跟随 host 演进的 sync 痛点 |
| 2 | **TimeSeriesStoreCapability** — 抽象出 history + daily rollup + retention | 第 3 个 metrics-like plugin 出现 |
| 3 | **RealtimePushCapability** — host SSE / WS 通道 | 任何 alerting / live-tail plugin 进入需求 |
| 4 | **加密密钥轮换 + 旧密文兼容**（W5 演进） | 真发生 key rotation 需求 |
| 5 | **JobScheduler 自动 retry / 死信队列 / priority**（W2 演进） | 出现长期 fail 的 job |
| 6 | **AuthSubjectCapability** 扩展 group_ids / api_key_id | 第二个需要 group 交集的 plugin |
| 7 | **DashboardWidgetSlot** | admin 抱怨"今日异常数要进子页才能看" |
| 8 | **AuditLogCapability** / **NotificationCapability** | 合规审计驱动 |

---

## 附录 A — Implementer 接 Step 模板

每个 sub-agent 接到 Step 后第一句话回答：

> 我接到 W{X}.{Y}，输入依赖 {列出}，预计改动 {N} 个文件 / {M} 行。我会先跑 `git log -1 --oneline` 验 base，再跑 `<验证命令>` 确保 baseline 干净，然后开工。完成后产 commit `<commit message>`，验证 `<验证命令>` 全绿后报告。

---

## 附录 B — 风险登记

| 风险 | 影响 | 缓解 |
|------|------|------|
| `vue-json-schema-form` (crickford) PoC 失败 | W3.4 阻塞 | fallback 到手写 `<DynamicSettingsForm/>`（已写在 D2） |
| W6 移植时发现 09fd83ab 的代码与 host 端 channel.go 接口已脱钩 | W6.4 编译失败 | 先跑 W8.2 vendor copy（拿到 channel.go 上下文）再跑 W6.4 |
| robfig/cron/v3 不解析 6 字段（带秒）cron | W2.3 cron job 触发不准 | 使用 `cron.New(cron.WithSeconds())` 显式启用秒字段 |
| AES-GCM nonce 重复（同 plugin key 跑 2³² 条） | 理论上密文可被破解 | 文档警示：单 plugin 加密 secret 数量预期 < 1M；超出走 V6 envelope encryption |
| plugin reset 后 SettingsExtension watcher 重连缓慢 | settings 变更延迟生效 | 退避上限 30s + Get 接口仍然走 RPC（不 stuck） |
| W3 LISTEN/NOTIFY 在 PG 重连时丢消息 | 短暂 inconsistency | SDK Watch 重连时强制走 GetAll 拉一次完整快照（已写入 §3.6） |
| W2 host 重启期间 cron job 错过触发 | 业务 SLA 受影响 | "缺触发不补"语义文档化；plugin 内提供 self-heal handler（如 monitor.run 自查最近 N 分钟未跑过） |
| W1 plugin 升级时新 migration 被跳过 | schema 不一致 | manifest checksum 校验 + 启动失败兜底（已写入 §1.6） |
