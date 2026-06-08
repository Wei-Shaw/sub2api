# Sub2API Plugin SDK

> 给外部插件作者的 SDK 使用文档。
> 主要针对 Go 编写的、与 Sub2API host 通过 gRPC 通信的进程外插件。

- 核心代码：`plugin-sdk/`（Go 模块 `github.com/Wei-Shaw/sub2api/plugin-sdk`）
- 示例插件：[`plugins/hello-world`](../plugins/hello-world/)（最小骨架 + Settings + Events）
  和 [`plugins/channel-management`](../plugins/channel-management/)（生产规模真实插件）
- 配套文档：
  [REDIS_API.md](REDIS_API.md) ·
  [SETTINGS_API.md](SETTINGS_API.md) ·
  [proto/sdk.proto](proto/sdk.proto)

---

## 目录

1. [概览 / Overview](#1-概览)
2. [架构 / Architecture](#2-架构)
3. [Quickstart](#3-quickstart)
4. [Manifest 字段表](#4-manifest-字段表)
5. [Capability 列表](#5-capability-列表)
6. [子客户端 / Subsystems](#6-子客户端--subsystems)
7. [CLI 接口 / Handshake](#7-cli-接口--handshake)
8. [Migrations（SQL 迁移）](#8-migrations)
9. [Frontend Bundle（前端资源）](#9-frontend-bundle)
10. [开发与测试 / Development](#10-开发与测试)
11. [版本与兼容性](#11-版本与兼容性)

---

## 1. 概览

`plugin-sdk` 是 Sub2API host 与插件进程之间的 **gRPC 桥**。
它替插件作者解决了：

- 与 host 的双向 gRPC 握手（host → plugin 生命周期；plugin → host 子客户端）
- DB / Redis / Settings / Events / Jobs / Secrets 等 host 资源的代理
- 进程级 SSRF 防护、远程日志、迁移分发、前端 bundle 流式传输
- traceparent 透传、`x-sub2api-plugin` 身份注入、capability gate

插件作者只需要：

```go
type MyPlugin struct{}
func (p *MyPlugin) Manifest() *pluginsdk.Manifest { /* … */ }
func (p *MyPlugin) Init(ctx pluginsdk.PluginContext) error { /* … */ }
func (p *MyPlugin) Shutdown() error { /* … */ }

func main() {
    if err := pluginsdk.Run(&MyPlugin{}); err != nil {
        log.Fatalf("plugin exited: %v", err)
    }
}
```

整个 gRPC 服务、HTTP 服务、信号处理、握手协议都由 `pluginsdk.Run` 接管。

---

## 2. 架构

```
┌──────────────────────────┐                   ┌──────────────────────────┐
│       Sub2API host       │                   │      Plugin process      │
│ (backend/internal/plugin │                   │  (your binary + SDK)     │
│         /manager.go)     │                   │                          │
│                          │                   │                          │
│  ┌──────────────────┐    │   PluginLifecycle │    ┌──────────────────┐  │
│  │ Manager          │ ─► │  Init / Shutdown  │ ─► │  pluginsdk.Run   │  │
│  │  - spawn(binary) │    │  GetManifest      │    │  - lifecycle srv │  │
│  │  - dial(grpc)    │    │  HealthCheck      │    │  - HTTP srv      │  │
│  └──────────────────┘    │  GetMigration     │    │  - your Plugin   │  │
│                          │  GetFrontendBundle│    └────────┬─────────┘  │
│  ┌──────────────────┐    │  ─── plus ───     │             │            │
│  │  SDK gRPC server │ ◄───── SQLProxy ──────────── ctx.DB()│            │
│  │                  │ ◄── RedisProxy ──────────── ctx.Redis()           │
│  │                  │ ◄── SecretEncryption ────── ctx.Secrets()         │
│  │                  │ ◄── JobScheduler ────────── ctx.Jobs()            │
│  │                  │ ◄── SettingsExtension ───── ctx.Settings()        │
│  │                  │ ◄── EventsExtension ─────── ctx.Events()          │
│  │                  │ ◄── LogProxy ────────────── slog handler          │
│  └──────────────────┘    │                   │                          │
└──────────────────────────┘                   └──────────────────────────┘
```

关键约定：

| 方向 | 服务 | 谁实现 | 何时调用 |
|------|------|--------|----------|
| host → plugin | `PluginLifecycle` | SDK runner | host 启动/停止 plugin、拉取 manifest / migration / 前端 bundle |
| host → plugin | `PricingExtension`（可选） | plugin 自行注册 | 网关计费、报价 |
| plugin → host | `SQLProxy` / `RedisProxy` | host | plugin 用 `ctx.DB()` / `ctx.Redis()` 时 |
| plugin → host | `SettingsExtension` / `EventsExtension` | host | plugin 用 `ctx.Settings()` / `ctx.Events()` 时 |
| plugin → host | `JobScheduler` | host | plugin Init 中 `ctx.Jobs().Register(...)`，host 反过来回调 |
| plugin → host | `LogProxy` | host | SDK 自动接管 `slog.Default()`，所有 log 都 stream 到 host |
| plugin → host | `SecretEncryption` | host | `ctx.Secrets().Encrypt/Decrypt`（host 持密钥） |
| plugin → host | `MigrationProxy` | host | 历史路径，主要由 `GetMigration` 取代 |

详细 proto 在 [`proto/sdk.proto`](proto/sdk.proto)。

---

## 3. Quickstart

最小可运行例子（基于 [`plugins/hello-world/main.go`](../plugins/hello-world/main.go)）：

```go
package main

import (
    "log"
    "net/http"

    pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
)

type HelloPlugin struct{}

func (p *HelloPlugin) Manifest() *pluginsdk.Manifest {
    return &pluginsdk.Manifest{
        Name:        "hello-world",
        DisplayName: "Hello World",
        Version:     "0.1.0",
        Author:      "you@example.com",
        IconSVG:     pluginsdk.IconPuzzle,

        // 声明 HTTP 路由 — host gateway 会把这些路径反代到本插件 HTTP 端口。
        PluginEndpoints: []pluginsdk.EndpointDecl{{
            Path:     "/api/v1/plugin/hello-world/hello",
            Methods:  []string{http.MethodGet},
            AuthType: pluginsdk.AuthTypeNone,
        }},
    }
}

func (p *HelloPlugin) Init(ctx pluginsdk.PluginContext) error {
    ctx.Logger().Info("hello-world plugin initialised")
    return nil
}

func (p *HelloPlugin) Shutdown() error { return nil }

// RegisterHTTP 是可选接口；声明 PluginEndpoints 时必须实现。
func (p *HelloPlugin) RegisterHTTP(mux pluginsdk.HTTPMux) {
    mux.Handle("/api/v1/plugin/hello-world/hello",
        http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            _, _ = w.Write([]byte(`{"message":"hello"}`))
        }))
}

func main() {
    if err := pluginsdk.Run(&HelloPlugin{}); err != nil {
        log.Fatalf("plugin exited: %v", err)
    }
}
```

启动方式（host 默认会用相同的 flag 列表 spawn 你的二进制；本地手测也可以这样起）：

```bash
go build -o ./bin/hello-world ./plugins/hello-world
./bin/hello-world --core-sdk-addr=127.0.0.1:51234 --log-level=debug
```

启动成功后插件会先输出一行 JSON handshake 到 stdout：

```json
{"protocol":1,"grpc_addr":"127.0.0.1:54321","http_addr":"127.0.0.1:54322","pid":12345}
```

host 读这行 JSON 后调用 `PluginLifecycle.Init` 完成接入。

---

## 4. Manifest 字段表

`Manifest` 完整定义在 [`manifest.go`](manifest.go)。常用字段：

| 字段 | 类型 | 说明 |
|------|------|------|
| `Name` | `string` | **唯一标识**，host 用作 redis 命名空间、capability 主体、SQL gate 主体。小写 + dash。 |
| `DisplayName` | `string` | 后台 UI 显示名。 |
| `Version` | `string` | 语义化版本字符串，host 把它写入 `plugin_settings.schema_version_at_write` 以检测漂移。 |
| `Description` / `Author` | `string` | 描述与作者，仅 UI 展示用。 |
| `GatewayEndpoints` | `[]EndpointDecl` | 接入 host 网关（`/v1/*`），通常用于 user-facing API。**需要 `http.register.gateway` capability。** |
| `PluginEndpoints` | `[]EndpointDecl` | 接入 host 内部 admin/管理路径（`/api/v1/plugin/<name>/*`）；默认 grant。 |
| `Frontend` | `*FrontendManifest` | 前端 bundle 入口、菜单项、Vue 路由（参考 §9）。 |
| `Migrations` | `[]MigrationDecl` | SQL 迁移（参考 §8）。声明后 host 自动开启 `migrations.apply` capability。 |
| `Capabilities` | `[]string` | 显式声明的特权能力（参考 §5）。 |
| `SettingsSchema` | `*SettingsSchemaDoc` | JSON Schema Draft-07 + defaults，admin UI 自动渲染表单；声明此字段会隐式开启 `settings.own.read`。 |
| `IconSVG` | `string` | 完整 `<svg>` 字符串；可用 `pluginsdk.IconPuzzle / IconBranchFork / IconCog`。 |
| `SubscribedEvents` | `[]string` | 订阅的 host 事件类型（参考 §6.5 Events）。 |
| `OwnedTables` | `[]string` | 插件拥有的 DB 表名；**P12·B-1 SQL gate 强制**：未在此列出且不在 host 共享白名单中的表查询会被拒。 |

### 4.1 EndpointDecl

```go
{
    Path:     "/api/v1/plugin/hello-world/db-test",
    Methods:  []string{http.MethodGet, http.MethodPost},
    AuthType: pluginsdk.AuthTypeAdmin, // admin / user / apikey / none
}
```

`AuthType` 决定 host 在转发请求前要校验的凭据类型，所有可选值见 `manifest.go` 中的 `AuthType*` 常量。

### 4.2 MenuItemDecl + Placement

```go
Frontend: &pluginsdk.FrontendManifest{
    EntryJS: "dist/entry.js",
    MenuItems: []pluginsdk.MenuItemDecl{{
        Path:    "/admin/plugins/hello-world",
        IconSVG: pluginsdk.IconPuzzle,
        Labels:  pluginsdk.Labels("Hello World", "Hello World"), // zh / en
        Section: pluginsdk.SectionAdmin,
        Placement: &pluginsdk.Placement{
            Group: pluginsdk.PlacementAdminEnd,
            Order: 100,
        },
    }},
},
```

`Placement` 是 V5/W7 引入的 DSL，决定菜单项落到 sidebar 哪一桶；不传则走 `SortOrder` 旧路径。

---

## 5. Capability 列表

唯一可信源在 [`capabilities.go`](capabilities.go) 的 `CapabilityRegistry`。
所有 capability 名都用 dotted-lowercase（`<resource>.<action>`），legacy snake_case 别名同时被识别。

### 5.1 Default-grant 类（声明即可，host 不会拒）

| Canonical | 用途 |
|-----------|------|
| `http.register.plugin` | 在 `/plugins/<name>/*` 注册 HTTP handler |
| `jobs.register`（旧名 `job_scheduler`） | 通过 `ctx.Jobs()` 声明定时任务 |
| `settings.own.read`（旧名 `settings_extension`） | 读自己的 settings；ship `SettingsSchema` 时隐式开启 |
| `settings.own.write` | 写自己的 settings（罕见；通常由 admin UI 写） |
| `events.subscribe.lowfreq` | 订阅低频 host 事件（如 `payment.order.created`） |
| `redis.own` | 自己的命名空间 redis 读写 |
| `db.own.read` / `db.own.write` | 读写 `OwnedTables` 列出的表 |
| `migrations.apply` | host 应用迁移；ship `Migrations` 时隐式开启 |

### 5.2 Declare-required 类（**必须**显式列在 `Capabilities`）

| Canonical | 用途 |
|-----------|------|
| `http.register.gateway` | 在 `/v1/*` 注册 gateway 路由（声明 `GatewayEndpoints` 时必需） |
| `events.subscribe.gateway`（旧名 `events.gateway`） | 订阅高频 gateway 事件，如 `gateway.model.invoked` |
| `secrets.encrypt`（旧名 `secret_encryption`） | 调用 `ctx.Secrets().Encrypt/Decrypt` |
| `outbound.http`（旧名 `safe_outbound_http`） | 通过 `pluginsdk.NewSafeHTTPClient` 出口 |
| `redis.raw`（旧名 `redis_raw_keys`） | `ctx.Redis().Raw()` 跨命名空间访问 |
| `db.core.read` | 读 host 共享白名单表（users / accounts / payment_orders 等） |
| `db.core.write` | 写共享表（**危险**，未来需 admin approve） |

> Legacy alias 在 host 端会被自动 normalize 成 canonical 名；同时 host 把已批准的 canonical 反展开成 legacy alias 推送给 plugin（`PluginInitRequest.capabilities`），保证旧插件升级 SDK 不会突然失效。

### 5.3 在代码里检查 capability

```go
// 例：events.go 内部判断 host 是否批了某个能力
if pluginsdk.CapabilityGrantedAny(approvedCaps, pluginsdk.CapabilityRedisRaw) {
    rdb = ctx.Redis().Raw()
}
```

详见 [`capabilities.go`](capabilities.go) 的 `CapabilityMatches` / `CanonicalCapability` / `LegacyAliasesFor`。

---

## 6. 子客户端 / Subsystems

`PluginContext` 的全部方法定义在 [`context.go`](context.go)。

### 6.1 `ctx.DB()` — SQL 代理

需要 `db.own.read` / `db.own.write`（默认 grant），并把表写进 `Manifest.OwnedTables`。

```go
var n int
err := ctx.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM channel_pricings").Scan(&n)
```

返回的是标准 `*sql.DB`，可直接交给 ent / gorm / sqlx；driver 实现见 [`driver/sql_driver.go`](driver/sql_driver.go)。

#### Using ent ORM

Plugin SDK 的 SQL driver 兼容 [ent](https://entgo.io/) ORM。
在 plugin 的 `Init` 中初始化 ent client：

```go
import (
    entsql "entgo.io/ent/dialect/sql"
    pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"
    "<your-plugin>/ent"
)

func (p *MyPlugin) Init(ctx pluginsdk.PluginContext) error {
    drv := entsql.OpenDB(pluginsdk.Dialect, ctx.DB())
    p.entClient = ent.NewClient(ent.Driver(drv))
    // ...
}
```

**注意事项**：
- 不要调用 `entClient.Schema.Create(ctx)` 做 auto migration。
  Plugin 使用声明式 SQL migration（`migrations/*.sql` + manifest 声明）。
- ent 的事务通过 SDK 的 gRPC SQL 代理透传，生命周期由 SDK 管理。
- `ctx.DB()` 返回的 `*sql.DB` 使用 gRPC driver，连接池参数
  （`MaxOpenConns` 等）控制的是 plugin 端的逻辑并发数，不直接对应
  host 端的物理连接数。

### 6.2 `ctx.Redis()` / `Raw()` — Redis 代理

默认所有 key 自动加前缀 `plugin:<name>:`；要写 host 共享 key 需声明 `redis.raw` 并 `ctx.Redis().Raw()`。

```go
rdb := ctx.Redis()
_ = rdb.SetEx(ctx, "smoke-test", "ok", 60*time.Second)
val, err := rdb.Get(ctx, "smoke-test")
```

完整命令表见 [REDIS_API.md](REDIS_API.md)。

### 6.3 `ctx.Settings()` — 配置读 + watch

需要 `settings.own.read`（ship `SettingsSchema` 时隐式开启）。

```go
// 读单值
var greeting string
if err := ctx.Settings().GetTyped(reqCtx, "greeting", &greeting); err == nil {
    fmt.Println(greeting)
}

// 监听变化
ch, cancel, err := ctx.Settings().Watch(ctx, "greeting")
defer cancel()
for change := range ch {
    log.Printf("greeting -> %s (rev=%d)", string(change.Value), change.Revision)
}
```

详见 [SETTINGS_API.md](SETTINGS_API.md) 与 [`settings.go`](settings.go) 中 `ErrSchemaVersionMismatch` 的处理建议。

### 6.4 `ctx.Jobs()` — 定时任务

需要 `jobs.register`（默认 grant）。**在 `Init` 内** 注册，`Init` 返回后 SDK 才会开 Subscribe stream。

```go
err := ctx.Jobs().Register(pluginsdk.JobSpec{
    Name:        "monitor.run",
    Trigger:     pluginsdk.JobTrigger{Kind: pluginsdk.TriggerInterval, Interval: 30 * time.Second},
    LeaderOnly:  true,
    Concurrency: 1,
    Timeout:     5 * time.Minute,
}, func(ctx context.Context, name string) error {
    return runOneCheck(ctx)
})
```

支持 `interval` / `cron` / `fixed_delay` 三种 trigger，详见 [`jobs.go`](jobs.go)。

### 6.5 `ctx.Events()` — 订阅 host 事件

事件类型必须先列在 `Manifest.SubscribedEvents`，否则 host 返回 `InvalidArgument`。
高频事件（`gateway.model.invoked`）额外要求 `events.subscribe.gateway` capability。

```go
// 在 Init 内：
p.eventsCtx, p.eventsCancel = context.WithCancel(context.Background())
err := ctx.Events().Subscribe(
    p.eventsCtx,
    []string{pluginsdk.EventTypeAuthUserRegistered},
    func(ctx context.Context, evt *pluginsdk.HostEvent) {
        reg := evt.GetAuthUserRegistered()
        if reg != nil {
            log.Printf("user registered: %s", reg.GetEmail())
        }
    },
)
```

T25 重构后 `Subscribe` 内部分两步：
- 同步 `ProbeSubscription` 校验 capability + 参数（错误立即返回）
- 异步 streamutil.Loop 跑 `Subscribe` 真正的流（指数退避 1s → 2s → 4s → 8s → 30s）

完整事件类型常量见 [`events.go`](events.go) 顶部的 `EventType*`。

### 6.6 `ctx.Secrets()` — 加解密

需要 `secrets.encrypt`。host 用 master key + 插件名做 HKDF-SHA256 派生子密钥；加密用 AES-256-GCM 且把插件名作为 AAD —— 因此 plugin A 的密文在 plugin B 那里解不开。

```go
ct, err := ctx.Secrets().Encrypt(reqCtx, []byte("api-key-xyz"))
pt, err := ctx.Secrets().Decrypt(reqCtx, ct) // 只有同一 plugin 能开
```

最大 plaintext 64KiB（`pluginsdk.MaxSecretBytes`）。详见 [`secrets.go`](secrets.go)。

### 6.7 `pluginsdk.NewSafeHTTPClient(cfg)` — 出口 HTTP

需要 `outbound.http`。返回的 `*http.Client` 默认拒绝 RFC1918 / 169.254.169.254 / 链路本地 / loopback / IPv6 ULA 等内网段；
`DialContext` 在每次拨号都重新解析+复检 IP，能扛住 DNS rebinding。

```go
client, err := pluginsdk.NewSafeHTTPClient(pluginsdk.OutboundConfig{
    AllowedHosts: []string{"api.openai.com"},
    Timeout:      15 * time.Second,
    MaxBodyBytes: 2 << 20, // 2 MiB
})
resp, err := client.Get("https://api.openai.com/v1/models")
```

零值 `OutboundConfig{}` 也能用，所有字段都有 fallback；详见 [`outbound.go`](outbound.go)。

### 6.8 `ctx.Logger()` — 自动接入 LogProxy

`ctx.Logger()` 返回的是 `*slog.Logger`，预先打了 `plugin=<name>` 标签。
SDK 在 `Init` 完成后会把 `slog.Default()` 也替换成同一个 handler，所以普通 `slog.Info(...)` 也会自动 stream 到 host。

stderr 在握手前用作 fallback；正常情况下所有日志都走 LogProxy，host 端聚合到 plugin 日志面板。

---

## 7. CLI 接口 / Handshake

插件是独立进程；host 通过 `--core-sdk-addr=<addr>` 把 SDK 反向连接地址告诉 plugin。
SDK 用 `flag` 解析（[`runner.go::parseFlags`](runner.go)）：

| Flag | 默认 | 说明 |
|------|------|------|
| `--core-sdk-addr` | `""` | host 端 SDK gRPC 服务地址；**host 启动 plugin 时必传** |
| `--grpc-listen` | `127.0.0.1:0` | plugin 自身的 gRPC 监听地址（lifecycle + 自定义 service） |
| `--http-listen` | `127.0.0.1:0` | plugin 自身的 HTTP 监听地址 |
| `--log-level` | `info` | `debug` / `info` / `warn` / `error` |
| `--no-http` | `false` | 不启动 HTTP 服务（pure-grpc plugin） |
| `--shutdown-wait` | `10s` | graceful shutdown 上限 |

启动后第一行 stdout 必须是握手 JSON：

```go
type Handshake struct {
    Protocol int    `json:"protocol"`  // 当前为 1（HandshakeProtocolVersion）
    GRPCAddr string `json:"grpc_addr"`
    HTTPAddr string `json:"http_addr"`
    PID      int    `json:"pid"`
}
```

host 读这一行后用 `grpc.NewClient(GRPCAddr, ...)` 拨号，下发 `Init`。

stderr 在 LogProxy 接管前是 SDK 的 fallback 日志通道；接管后 stderr 不再活跃。

进程会监听 `SIGINT` / `SIGTERM`，host 调 `Shutdown` 也会触发同样的 graceful 退出路径（`runner.gracefulShutdown`）。

---

## 8. Migrations

声明 `Migrations` 时插件必须实现 `MigrationProvider` 接口（通常基于 `embed.FS`）：

```go
//go:embed migrations/*.sql
var migrationFS embed.FS

func (p *MyPlugin) OpenMigration(filename string) ([]byte, error) {
    return migrationFS.ReadFile("migrations/" + filename)
}

// Manifest:
Migrations: []pluginsdk.MigrationDecl{{
    Filename:           "001_create_channels.sql",
    ChecksumSha256:     "abcdef...64hex",
    NonTransactional:   false,
    DownFilename:       "001_create_channels.down.sql",
    DownChecksumSHA256: "012345...64hex",
}},
```

要点：

- 文件按 `Filename` 字典序应用；**checksum 是不可变的**（appendix-only），改了 host 会拒绝 plugin 启动以防供应链篡改。
- `NonTransactional=true` 用于 `CREATE INDEX CONCURRENTLY` 这类不能在 BEGIN/COMMIT 里跑的语句。
- 提供 `DownFilename` 才能在 `Plugin Purge` 时回滚；为空表示不可逆迁移。
- host 端 schema 在 `plugin_migrations` 表，回滚记录 / drift 检测都在那里。

---

## 9. Frontend Bundle

如果插件想注册管理界面 / 用户视图，要：

1. 在仓库里写 Vue 前端（参考 [`plugins/hello-world/frontend`](../plugins/hello-world/frontend)），打包到 `dist/`。
2. 通过 `embed.FS` 嵌入二进制，并实现 `FrontendBundleProvider`：

```go
//go:embed all:frontend/dist
var frontendAssets embed.FS

func (p *HelloPlugin) OpenFrontendFile(rel string) ([]byte, error) {
    clean := path.Clean("/" + rel)
    if clean == "/" || clean == "/." {
        return nil, fs.ErrInvalid
    }
    return frontendAssets.ReadFile("frontend/" + clean[1:])
}
```

3. 在 `Manifest.Frontend` 声明入口 + 菜单 + 路由：

```go
Frontend: &pluginsdk.FrontendManifest{
    EntryJS:  "dist/entry.js",
    EntryCSS: "dist/entry.css",
    MenuItems: []pluginsdk.MenuItemDecl{ /* … */ },
    Routes:   []pluginsdk.RouteDecl{ /* … */ },
    I18nNamespaces: []string{"helloWorldPlugin"},
},
```

host 通过 `PluginLifecycle.GetFrontendBundle` 流式拉取（每 chunk 64 KiB），代理到 `/api/v1/plugin-assets/<name>/...`。

---

## 10. 开发与测试

### 10.1 本地起插件

```bash
# 直接 go run（host 没起的话，--core-sdk-addr 任意填，Init 会失败但能看 manifest）
go run ./plugins/hello-world --core-sdk-addr=127.0.0.1:0 --log-level=debug

# 或编译再起
go build -o ./bin/hello-world ./plugins/hello-world
./bin/hello-world --core-sdk-addr=127.0.0.1:51234
```

### 10.2 单元测试

SDK 自带的测试 helper：

- [`jobs_testhelpers_test.go`](jobs_testhelpers_test.go) — fake JobScheduler stream，用来测 plugin 的 job handler
- [`settings_test.go`](settings_test.go) / [`events_test.go`](events_test.go) / [`outbound_test.go`](outbound_test.go) — 可参考的 fixture 写法
- [`runner_migration_test.go`](runner_migration_test.go) — `MigrationProvider` 接口测试模板

最小测试模板（在 plugin 包内）：

```go
//go:build unit

func TestMyHandler(t *testing.T) {
    p := &MyPlugin{}
    if err := p.Init(fakePluginContext(t)); err != nil {
        t.Fatal(err)
    }
    // … 调具体方法 …
}
```

### 10.3 集成测试

把 hello-world 当模板。host 端有 `proxy_smoke_test.go` 覆盖 SQL/Redis 代理；插件可以在 `cmd/<name>` 下加 e2e harness 用 `go test -tags=integration` 跑。

---

## 11. 版本与兼容性

- **proto 兼容**：`proto/sdk.proto` 头部维护 changelog（如 T24 EventsExtension 的 ProbeSubscription 拆分、T25 Watch 取消语义）。proto 字段一律 append-only；废弃字段保留 number 不复用。
- **SDK 与 host 同步升级**：当前实现假设 SDK / host 同代码库构建（同一 git commit）。跨版本运行时会通过 `HandshakeProtocolVersion` 检测；不匹配时 host 拒绝接入，避免半破半通的状态。
- **Capability legacy alias 一释放周期**：每个 legacy snake_case 名（`redis_raw_keys`、`secret_encryption`、`settings_extension`、`job_scheduler`、`events.gateway`）都会保留至少一个 release 周期，期间 host 同时 normalize 进、expand 出，老 plugin 二进制不动也能继续跑。
- **manifest schema 升级**：`Manifest.SettingsSchema.Version` 在 host `plugin_settings.schema_version_at_write` 留痕；plugin 升级 SDK 后用 `errors.Is(err, pluginsdk.ErrSchemaVersionMismatch)` 检测漂移并按需做迁移。

---

## 参考资料

- [hello-world plugin 全部源码](../plugins/hello-world/main.go) — 最小可运行例子
- [channel-management plugin](../plugins/channel-management/) — 真实生产规模插件，覆盖了几乎所有 capability
- [`proto/sdk.proto`](proto/sdk.proto) — gRPC wire 契约
- [`docs/plugin-architecture/`](../docs/plugin-architecture/) — 设计文档（V5 设计、SETTINGS-V2、PLUGIN-EVENTS 等）
- 历次重构记录：`PLUGIN_SYSTEM_HANDOFF.md`（worktree 根目录）

如发现文档与代码漂移，请以 SDK 源码（`plugin-sdk/*.go` 注释）为准并提 issue / PR。
