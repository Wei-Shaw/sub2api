# 内建层插件作者指南（Builtin Plugin Author Guide）

> 适用范围：`backend/internal/plugins/` 下的**内建层插件**（进程内、编译期装配）。
> 唯一权威接口：`backend/internal/pluginkit/{plugin,host,router,state}.go`——本指南与代码不一致时以代码为准。
> 专项全局规划见 [.claude/plugin-system/ROADMAP.md](../../.claude/plugin-system/ROADMAP.md)，设计决策见同目录 PROPOSAL.md。

本指南以 `internal/plugins/demo/demo.go` 为教学主线——它是覆盖全部扩展面（Initializer / Runner / APIProvider）的最小完整样例，可直接作为新插件的起点模板。文中代码片段均摘自真实实现。

## 目录

1. [核心契约](#1-核心契约)
2. [Host 能力面与纪律](#2-host-能力面与纪律)
3. [私有配置子树](#3-私有配置子树)
4. [路由挂载与分发路径](#4-路由挂载与分发路径)
5. [启停语义](#5-启停语义)
6. [kittest 用法与必测清单](#6-kittest-用法与必测清单)
7. [demo 全流程演练](#7-demo-全流程演练)
8. [提交前自检清单](#8-提交前自检清单)

---

## 1. 核心契约

### 1.1 最小契约与可选扩展面

插件只有一个必选接口，能力经可选接口声明（定义见 `pluginkit/plugin.go`）：

```go
type Plugin interface {
    ID() ID
}

// 可选：需要依赖注入与配置校验时实现
type Initializer interface {
    Init(ctx context.Context, host *Host) error
}

// 可选：后台常驻型插件实现
type Runner interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}

// 可选：自带 HTTP API 的插件实现
type APIProvider interface {
    MountRoutes(r Router)
}
```

在插件包内用编译期断言声明你实现的扩展面（demo 的写法）：

```go
var (
    _ pluginkit.Plugin      = (*Plugin)(nil)
    _ pluginkit.Initializer = (*Plugin)(nil)
    _ pluginkit.Runner      = (*Plugin)(nil)
    _ pluginkit.APIProvider = (*Plugin)(nil)
)
```

### 1.2 ID 规则

点分命名空间风格（如 `demo`、`job.backup`），与前端插件、外部插件共享同一命名空间。校验规则（`ID.Validate`）：小写字母/数字，段间以 `.` 或 `-` 分隔，长度 1-64，正则 `^[a-z0-9]+([.-][a-z0-9]+)*$`。命名空间规划见 ROADMAP。

### 1.3 生命周期四条铁律

Manager（`pluginkit/manager.go`）驱动生命周期：`Factory 构造（仅一次）→ [enabled] Init → Start → ... → Stop`。插件作者必须遵守：

1. **构造无副作用**：`Factory` 只分配内存。不开 goroutine、不连数据库、不读配置——一切资源获取与校验放在 `Init`。列入装配清单 ≠ 产生行为：未启用的插件不 Init、不 Start。

   ```go
   // New 是 demo 插件的 Factory，签名必须匹配 pluginkit.Factory。
   // 只分配内存，无任何副作用。
   func New() pluginkit.Plugin { return &Plugin{} }
   ```

2. **Init 可重入**：`Init` 必须可在 `Stop` 之后再次调用。enable→disable→enable 复用**同一实例**（见 1.4），重启即同一对象重新 Init+Start。Init 失败该插件进 `failed` 态并隔离，不拖垮宿主与其他插件。

3. **Stop 完全回收**：`Start` 拉起的全部后台资源（goroutine / ticker / 连接）必须由 `Stop` 回收，且 `Stop` 返回后实例可被再次 Init/Start。`Stop` 受 ctx deadline 约束（宿主默认给 10s，超时记 `failed` 并继续，不阻塞进程退出）。未启动时 `Stop` 应幂等返回 nil。

4. **后台 goroutine 不挂在 Start 的 ctx 上**：那是宿主的启动上下文。正确姿势是插件自持 cancel、在 Stop 中显式终止并等待退出。demo 的模式：

   ```go
   func (p *Plugin) Start(_ context.Context) error {
       p.mu.Lock()
       defer p.mu.Unlock()
       if p.cancel != nil {
           return errors.New("demo: already started")
       }
       ctx, cancel := context.WithCancel(context.Background())
       p.cancel = cancel
       p.done = make(chan struct{})
       p.startedAt = time.Now()
       // 心跳所需数据以参数传入，goroutine 不回读实例字段，避免与重启期间的写入竞争。
       go heartbeat(ctx, p.done, p.host.Logger, p.cfg)
       return nil
   }

   func (p *Plugin) Stop(ctx context.Context) error {
       p.mu.Lock()
       cancel, done := p.cancel, p.done
       p.cancel, p.done = nil, nil
       p.mu.Unlock()
       if cancel == nil {
           return nil
       }
       cancel()
       select {
       case <-done:
           return nil
       case <-ctx.Done():
           return fmt.Errorf("demo: waiting for heartbeat to exit: %w", ctx.Err())
       }
   }
   ```

### 1.4 同实例重启与并发边界

- 生命周期方法（Init/Start/Stop）由 Manager **按插件串行**调用（per-plugin mutex），插件内部无需为它们之间的互相竞争加锁；
- 但 **HTTP handler 可能与下一轮 Init 并发**（在途请求 vs 重启），可变字段不能裸读——demo 用一把 `sync.Mutex` 保护 `host/cfg/startedAt/cancel/done`；
- 未 Stop 的重复 Start 应报错而非静默泄漏上一轮 goroutine（demo 的 `already started` 检查）。

### 1.5 装配清单

`internal/plugins/builtin.go` 是**唯一编译期装配清单**。新增内建插件 = 在此追加一行 Factory；禁止 `init()` 自注册等隐式装配；清单顺序不承载语义（Manager 按 ID 稳定排序）：

```go
func Builtin() []pluginkit.Factory {
    return []pluginkit.Factory{
        demo.New,
    }
}
```

内建插件之间**禁止互相 import**；协作需求走核心 ports/事件（届时在 PROPOSAL 登记）。

### 1.6 默认启用（DefaultEnabler）与插件边界口径

插件默认状态是"未启用"（无 `plugin_states` 记录 = disabled）。但**从既有常驻功能迁移来的插件**（迁移前一直在运行）迁移后必须默认开启以零行为变更。实现可选接口：

```go
type DefaultEnabler interface{ DefaultEnabled() bool }
```

语义（Manager.Bootstrap 自播种）：仅当该 ID **从无显式记录**时，落一条 `enabled=true`（`updated_by=pluginkit:default-enabled`）并随 Bootstrap 启动；此后一切以状态存储为准——**管理员显式停用不会被再次翻开**（重启仍停用）。

**插件边界口径（用户决策，回退记录）**：插件 = **按功能域整体竖切**（一个完整业务功能 = 一个插件，见 §1.7）。基础设施型后台 worker（账号/代理过期扫描、幂等清理等）**不是插件**——它们没有独立的业务功能面，不应出现在插件管理页；其常驻生命周期由 Wire 直接管理（构造期 Start + provideCleanup 停止块）。早期把此类 worker 收编为 `job.*` 插件壳的做法已整体回退，勿再照搬。

### 1.7 功能域竖切（原子句柄模式）

**按功能模块整体收编**（一个完整业务功能 = 一个插件，含其 worker/API/前端门控）时，功能往往被热路径或既有静态路由持有，无法像纯 worker 那样只在 Wire 摘一刀。参考实现：内容审计 → `content-moderation` 插件（`internal/plugins/moderation/`）。固定工序：

1. **原子句柄建缝**：在 service 包加 `XxxHandle`（`atomic.Pointer[XxxService]`，Bind/Unbind/Get）。持有方（网关/后台 handler）构造期注入句柄、**每请求 Get() 解析**：nil = 功能熄灭。热路径调用点若已有 nil 守卫（推荐先特征化锁定），停用即直通，与"服务不存在"同语义；
2. **补最小生命周期**：若服务的后台 goroutine 在构造函数裸启动且无 Stop，补 `stopCh/WaitGroup/baseCtx` 与 `Stop(ctx)`（在途外呼随 baseCtx 中止）——这是 Stop 完全回收的硬门槛，不属于"顺手优化"；
3. **插件壳**：Runner + DefaultEnabler；Start 新建服务并 `handle.Bind`，Stop 先 `Unbind`（新请求立即熄灭）再 `svc.Stop(ctx)`；已解析到旧实例的在途请求自然完成（要求异步入队非阻塞）；
4. **后台端点熄灭**：admin handler 每请求解析句柄，nil → 404（对齐分发器语义）；**原 API 路径保持不变**（路由表零变化）；
5. **前端门控（原位收编）**：静态路由手写 `meta.pluginId` 接入插件守卫、导航项在原 featureFlag 之上叠加 `enabledPluginsStore.isEnabled(id)`——菜单位置/i18n 不动，规避视觉与行为变化；
6. 守护测试登记：前端路由守护测试的 `PLUGIN_GATED_CORE_ROUTES` 清单需登记新收编路由；
7. **物理搬家（单独 PR）**：后端 `git mv` 域文件进 `internal/plugins/<域>/`（package 随迁），留守标识符加 `service.` 前缀、域内类型随迁、消费方改引插件包；**方向纪律：service 永不 import 插件包**——若域代码触碰了 service 未导出内部（如内容审计的通知邮件管线），在 service 侧加最小导出出口（如 `EmailService.SendNotificationEmailWithFallback`），行为逐行等价；耦合了域内未导出成员的留守测试随迁。前端 `git mv` View/spec/api 进 `src/plugins/<域>/`，`api/admin/index.ts` 保持转发出口使其余消费方零改动。验收：git 重命名相似度高位 + 路由表逐条 diff 一致 + 随迁测试全绿。

### 1.8 插件设置面板（前端设置入口）

插件的管理员设置界面统一挂在**插件管理页**（/admin/plugins）卡片的"设置"入口，不往核心 System Settings 页塞分区（对齐 WordPress / Grafana 等的 per-plugin settings page 通例）。机制：

- 前端描述符（`src/pluginkit/types.ts` 的 `PluginDescriptor`）可选声明 `settings: () => import('./SettingsPanel.vue')`——懒加载，未打开弹窗不加载插件 chunk（与路由组件同纪律）；
- 插件管理页经 `pluginSettingsLoader(id)`（`src/pluginkit/registry.ts`，内建与运行时外部插件同一查找）渲染"设置"按钮与弹窗；**入口的有无完全由描述符声明驱动，宿主不对任何插件 ID 特判**；
- 面板组件自包含：自行加载/保存自己的配置（走插件自己的 API 或既有核心 API），宿主只提供弹窗容器；文案放 `plugins.<id>.*` 命名空间（插件 i18n 贡献机制），zh/en 键集必须一致；
- 外部插件的 JSON 配置编辑器（manifest `config_schema` 驱动）保持不变，与设置面板并存：声明了 settings 的插件额外获得富面板入口。

参考实现：`src/plugins/content-moderation/`（index.ts 描述符 + SettingsPanel.vue + i18n.ts）——风控总开关与 cyber 会话屏蔽设置自 System Settings 风控分区迁入，原分区已删除。

---

## 2. Host 能力面与纪律

每个插件在 `Init` 时收到自己的 `*pluginkit.Host`（`pluginkit/host.go`），这是插件能拿到的**全部**宿主能力：

```go
type Host struct {
    Logger *slog.Logger
    DB     *ent.Client
    Redis  *redis.Client
    // Config 解码本插件在 plugins.<id> 下的私有配置子树；未配置时对 out 零值填充并返回 nil。
    Config func(out any) error
}
```

使用要点：

- **Logger** 已自动携带 `plugin=<id>` 字段，直接用即可，输出结构化日志（`logger.Info("demo_heartbeat", "greeting", cfg.Greeting)`）；
- **DB / Redis** 是宿主共享的客户端；不需要数据库的插件测试可以拿到 nil（kittest 默认 nil），生产环境总是非 nil；
- **Config** 见第 3 节。

**能力面扩展纪律**：每新增一个 Host 字段必须在 PROPOSAL 的 Host 能力登记表补一行并说明理由；**永不暴露具体 Service**——插件需要业务能力时先抽 ports 接口再挂载。插件侧对应的纪律：不要绕过 Host 去 import `internal/service` / `internal/repository` 等宿主内部包。

---

## 3. 私有配置子树

### 3.1 职责边界

配置文件（`backend/config.yaml`）的 `plugins:` 子树**只承载插件私有配置**；`enabled` 状态不在配置文件中（唯一事实源是 DB，见第 5 节）。yaml 写法：

```yaml
plugins:
  demo:
    greeting: "hola"
    interval: "10s"
```

### 3.2 插件侧用法（demo 模式）

字段用 `mapstructure` 标签；在 `Init` 中**先填默认值再解码**（配置中未出现的字段保持默认），解码后立即校验：

```go
type Config struct {
    Greeting string        `mapstructure:"greeting"`
    Interval time.Duration `mapstructure:"interval"`
}

func (p *Plugin) Init(_ context.Context, host *pluginkit.Host) error {
    cfg := Config{
        Greeting: defaultGreeting,
        Interval: defaultInterval,
    }
    if err := host.Config(&cfg); err != nil {
        return err
    }
    if strings.TrimSpace(cfg.Greeting) == "" {
        return errors.New("demo: greeting must not be blank")
    }
    if cfg.Interval <= 0 {
        return fmt.Errorf("demo: interval must be positive, got %s", cfg.Interval)
    }
    // ...
}
```

语义约定（`pluginkit/config.go` 的 `DecoderFor`）：

- **未配置该插件** → `host.Config` 对 out 不做任何写入并返回 nil（零值成功）——插件必须能在"整段缺省 = 全默认"下正常工作；
- **显式配置了非法值** → 在 Init 中报错（进 failed 态），不要静默回退默认；
- 解码语义对齐主配置（viper 默认行为）：弱类型转换 + duration 字符串（`"30s"`、`"1m"`）+ 逗号分隔 slice 字符串。

### 3.3 viper 含点 key 的拆层问题

viper 对含点 key 按 `.` 拆层存储（`plugins.job.backup.greeting` 变成 `job→backup→greeting` 嵌套 map），`ParsePluginsConfig` 按以下规则还原点分插件 ID：

1. key 本身含点（yaml 字面 key，如 `"job.backup":`）→ 视为完整插件 ID，其值即私有配置，不再下钻；
2. key 不含点且值是"纯命名空间层"（非空 map 且所有子值均为 map）→ 下钻一层，路径段以 `.` 拼接；
3. 其余情况（值含标量叶子、或为空 map / nil）→ 当前路径即插件 ID。

**歧义与规避**：规则 2 无法区分"命名空间层"与"私有配置顶层恰好全是嵌套 map"的插件（如插件 `job` 的配置只有 `limits: {max: 5}` 会被误还原为插件 `job.limits`）。插件作者两种规避方式任选其一：

- yaml 中用**字面点分 key** 书写（`"job.backup":`，走规则 1，永不歧义）；
- 保证私有配置**顶层含标量字段**。

兜底：`Manager.Bootstrap` 对未匹配任何注册插件的配置 ID 打 `plugin_config_unknown_id` warn 日志；同一 ID 经字面 key 与拆层两种形态同时出现直接报错（拒绝启动），不静默合并。

---

## 4. 路由挂载与分发路径

### 4.1 插件侧：只写相对路径

实现 `APIProvider` 后，`MountRoutes` 仅在宿主 Bootstrap 时调用一次。插件在私有子路由器上注册**相对路径**（`pluginkit/router.go`）：

```go
func (p *Plugin) MountRoutes(r pluginkit.Router) {
    r.Admin().GET("/hello", p.handleHello)
}
```

- `r.Admin()`：管理员面，对外 `ANY /api/v1/admin/plugins/:id/api/*path`，宿主套 AdminAuth + 合规守卫；
- `r.User()`：用户面，对外 `ANY /api/v1/plugins/:id/api/*path`，宿主套 JWT 认证 + 后台模式守卫。

鉴权中间件与启停门控由宿主分发器统一施加，**插件无法绕过，也无需（不能）自查 enabled 状态**。两个面互相隔离：Admin 面注册的路由不会出现在 User 面。

### 4.2 分发路径（宿主侧，了解即可）

插件 API 走 `:id` 分发器而非静态注册（规避 gin 路由树 static/param 冲突，门控集中，与外部层分发同构）。请求链路：

```
GET /api/v1/admin/plugins/demo/api/hello
 → 宿主 gin：AdminAuth + 合规守卫（routes/plugin.go）
 → Manager.Dispatch(SideAdmin, "demo", c)
    ├─ 门控：GateAllows = enabled（内存快照）且 state==running
    ├─ 未知 ID / 未启用 / 非 running / 无 API 面 → 同一个 404（不区分原因，避免探测）
    └─ 路径重写为 /admin/hello，转发到插件私有 gin.Engine
 → 插件 handler 自行写响应（不经宿主统一响应包装）
```

注意：插件 handler 的响应体**不套** `{"code":0,...}` 宿主信封，demo 的 `/hello` 直接返回 `{"greeting":...,"uptime_seconds":...}`。插件内部未注册的路径由子路由器返回 404。

---

## 5. 启停语义

### 5.1 事实源与免重启

- **enabled 的唯一事实源是 DB**（`plugin_states` 表，迁移 `170_plugin_states.sql`）；**无行 = disabled**。配置文件不参与启停判定；
- 新插件列入 `builtin.go` 清单后默认 disabled：不 Init、不 Start、API 404，零行为变更；
- 启停经 admin API **免重启**生效：

  ```
  GET  /api/v1/admin/plugins                插件状态快照（按 ID 稳定排序）
  POST /api/v1/admin/plugins/:id/enable     启用（未注册 ID → 404；重复启用幂等 200）
  POST /api/v1/admin/plugins/:id/disable    停用（同上）
  ```

- 多实例部署：变更经 Redis Pub/Sub（频道 `plugin:state:changed`）跨实例广播，60s 定时对账兜底广播丢失；本实例的生效不依赖广播链路（SetEnabled 返回后快照即为最新状态）。

### 5.2 状态机

`PluginStatus.State` 四态（`pluginkit/plugin.go`）：

| 状态 | 含义 | 门控 |
|:--|:--|:--|
| `disabled` | 未启用（无实例活动） | 关 |
| `running` | 已启用且 Init/Start 成功 | **开** |
| `failed` | Init/Start 失败，或 Stop 超时 | 关 |
| `stopped` | 曾运行、已停用且 Stop 成功（与 disabled 仅历史之别） | 关 |

对插件作者的含义：

- Init/Start 返回 error → 该插件 `failed` 并隔离，宿主与其他插件正常——所以校验要放 Init，宁可 failed 也不要带病 running；
- 分发器门控是 `enabled && running`：failed 态即使 enabled，API 也是 404；
- disable 时 Manager 调用你的 Stop（默认 10s 超时）；Stop 报错/超时 → `failed`（现场保留便于排障）；
- 再次 enable = 同一实例重新 Init → Start（第 1 节契约的意义所在）。

---

## 6. kittest 用法与必测清单

`internal/pluginkit/kittest/` 提供共享测试夹具（`//go:build unit`，不进生产构建）。**所有插件测试文件必须带 `//go:build unit` 标签**，运行方式：

```bash
cd backend
go test -tags=unit ./internal/plugins/<yourplugin>/
```

### 6.1 夹具速览

```go
// 内存版 Host：Logger 输出到 t.Logf（测试结束自动静音），DB/Redis 默认 nil，
// Config 默认零值成功（等价于未配置该插件）。
host := kittest.NewHost(t)

// 可选项：WithDB(entClient) / WithRedis(rdb) / WithLogger(logger) / WithConfig(decode)
host := kittest.NewHost(t, kittest.WithConfig(cfg.DecoderFor(PluginID)))

// 纯内存 StateStore（无 DB/广播；回调同步交付，SetEnabled 返回后即可确定性断言）
store := kittest.NewMemoryStateStore()

// 启停循环泄漏断言：cycles 轮 enable→disable，断言每轮 running/stopped 收敛、
// 门控开关正确、goroutine 数回落基线、终态 Snapshot 干净。
kittest.AssertToggleCycleClean(t, m, store, PluginID, 3)
```

走真实配置解码链路的推荐写法（摘自 demo 测试）：

```go
func decoderFor(t *testing.T, conf map[string]any) func(out any) error {
    t.Helper()
    cfg, err := pluginkit.ParsePluginsConfig(map[string]any{"demo": conf})
    require.NoError(t, err)
    return cfg.DecoderFor(PluginID)
}
```

用真实 Manager 装配做全链路测试（摘自 demo 测试）：

```go
store := kittest.NewMemoryStateStore()
deps := pluginkit.HostDeps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
m, err := pluginkit.NewManager(deps, store, cfg, []pluginkit.Factory{New})
require.NoError(t, err)
require.NoError(t, m.Bootstrap(context.Background()))
t.Cleanup(func() { require.NoError(t, m.StopAll(context.Background())) })
```

### 6.2 必测清单

对照 `internal/plugins/demo/demo_test.go`（每一项都有现成范例可抄）：

- [ ] **配置解码**：整段缺省 → 全默认；部分配置 → 未配置字段保持默认；每个校验分支的非法值 → Init 报错且错误信息可定位；
- [ ] **Runner 启停**（如实现）：Start 后按预期产生后台行为；Stop 后行为完全停止（等待数倍周期再断言）；重复 Stop / 未启动时 Stop 幂等；未 Stop 的重复 Start 报错；
- [ ] **同实例重启**：Stop 之后同一实例重新 Init → Start，行为恢复正常；
- [ ] **API 全链路**（如实现，经 `Manager.Dispatch` 而非直挂 handler）：未启用 → 404；启用 → 200 且响应体正确；admin/user 面隔离；插件内未注册路径 → 404；disable → 404；再次 enable → 恢复；
- [ ] **启停循环泄漏断言**：`kittest.AssertToggleCycleClean(t, m, store, PluginID, 3)`（≥3 轮，阶段验收硬性标准）。

需要 Redis 的插件用 miniredis + `kittest.WithRedis`；需要 DB 的用 enttest + `kittest.WithDB`（先 grep 相邻测试找现成模式）。

---

## 7. demo 全流程演练

以下演练验证"免重启启停"全链路，全程不重启进程。

**Step 0 — 配置（可选）**：`backend/config.yaml` 加私有配置（不加则全默认）：

```yaml
plugins:
  demo:
    greeting: "hola"
    interval: "10s"
```

**Step 1 — 启动**：`go run ./cmd/server`。启动时自动应用迁移（含 `170_plugin_states.sql` 建表）；demo 已在 `builtin.go` 清单中但 DB 无行 = disabled，无任何行为。

**Step 2 — 看快照**（admin 鉴权二选一：`x-api-key: <admin-api-key>` 或 `Authorization: Bearer <admin-jwt>`）：

```bash
curl -s -H "x-api-key: $ADMIN_KEY" http://127.0.0.1:8080/api/v1/admin/plugins
# → {"code":0,"message":"success","data":[{"id":"demo","enabled":false,"state":"disabled"}]}
```

**Step 3 — 未启用时 API 是 404**：

```bash
curl -s -H "x-api-key: $ADMIN_KEY" http://127.0.0.1:8080/api/v1/admin/plugins/demo/api/hello
# → 404 plugin not found
```

**Step 4 — enable（免重启）**：

```bash
curl -s -X POST -H "x-api-key: $ADMIN_KEY" http://127.0.0.1:8080/api/v1/admin/plugins/demo/enable
# → data 即该插件最新快照：{"id":"demo","enabled":true,"state":"running","started_at":"..."}
```

服务端日志出现 `plugin_started plugin=demo`，随后每 interval 一条心跳：`demo_heartbeat plugin=demo greeting=hola`。

**Step 5 — 调插件 API**：

```bash
curl -s -H "x-api-key: $ADMIN_KEY" http://127.0.0.1:8080/api/v1/admin/plugins/demo/api/hello
# → {"greeting":"hola","uptime_seconds":42}   （插件自写响应，无宿主信封）
```

**Step 6 — disable**：

```bash
curl -s -X POST -H "x-api-key: $ADMIN_KEY" http://127.0.0.1:8080/api/v1/admin/plugins/demo/disable
# → {"id":"demo","enabled":false,"state":"stopped"}
```

心跳日志停止（`plugin_stopped plugin=demo`），Step 5 的路径回到 404。再次 enable 即恢复（同一实例重新 Init+Start）。

**故障注入**：把 `interval` 配成 `"-1s"` 后 enable → 快照变为 `{"state":"failed","error":"demo: interval must be positive, got -1s"}`，API 404，宿主与其他插件不受影响。

---

## 8. 提交前自检清单

**契约**

- [ ] Factory 无副作用（只分配内存）；ID 通过 `Validate` 且已在 ROADMAP 命名空间规划内；
- [ ] Init 可在 Stop 后再次调用；显式配置非法值时 Init 报错，整段缺省时全默认可运行；
- [ ] Stop 完全回收（goroutine/ticker/连接），幂等，尊重 ctx deadline；后台 goroutine 不挂在 Start 的 ctx 上；
- [ ] HTTP handler 与重启期 Init 的并发已用锁保护（不裸读可变字段）；
- [ ] 编译期断言声明了全部实现的扩展面。

**边界**

- [ ] 只经 Host 获取依赖，未 import service/repository 等宿主内部包；未 import 其他内建插件；
- [ ] 需要新 Host 能力时已在 PROPOSAL 能力登记表登记并评审；
- [ ] MountRoutes 只注册相对路径，未自查 enabled、未自套鉴权；
- [ ] 已在 `internal/plugins/builtin.go` 追加 Factory（唯一装配点），无 init() 自注册。

**测试与质量**

- [ ] 测试文件带 `//go:build unit`；第 6.2 节必测清单全覆盖；`AssertToggleCycleClean` ≥3 轮通过；
- [ ] `go test -tags=unit ./internal/plugins/<yourplugin>/` 全绿；
- [ ] `gofmt` / `go vet -tags=unit` 干净；无新第三方依赖；
- [ ] 结构化日志（slog key-value），无 fmt.Println；错误全部处理或上抛；
- [ ] 不启用你的插件时行为与主干完全一致（零行为变更）。
