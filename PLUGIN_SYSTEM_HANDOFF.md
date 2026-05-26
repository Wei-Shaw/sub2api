# Sub2API 插件系统 — 完整交接文档

## 一、项目背景

**项目**: Sub2API — Go + Vue 3 的 AI API 网关，将多平台（Anthropic/OpenAI/Gemini）账号聚合为统一 API 端点，支持用户管理、计费、渠道定价等。

**技术栈**:
- 后端: Go 1.26.2 + Gin + Ent ORM + PostgreSQL + Redis + Google Wire DI
- 前端: Vue 3 + TypeScript + Vite + Vue Router + Pinia + Vue I18n
- 模块路径: `github.com/Wei-Shaw/sub2api`
- 前端编译后 embed 进 Go 二进制

**工作分支**: `claude/wizardly-lumiere-b4ad6c`（git worktree）
**工作目录**: `C:\Users\user\project\GolandProjects\sub2api\.claude\worktrees\wizardly-lumiere-b4ad6c`

---

## 二、目标

将"渠道管理"功能（渠道定价、渠道监控、可用渠道、渠道状态）从核心代码中抽取为独立的**插件**。设计了一套通用插件系统，插件作为**独立进程**运行，通过 **gRPC** 与核心通信。

### 核心架构原则

1. **单向依赖**: 插件 → SDK → 核心。核心永远不调用插件业务 API
2. **核心是纯基础设施**: 路由代理、认证中间件、插件生命周期管理。零业务感知
3. **插件全栈自闭环**: 独立进程、自有后端逻辑、前端 bundle、数据库表
4. **gRPC 独立进程**: 插件编译为独立二进制，通过 gRPC 与核心通信
5. **共享数据库**: SDK 提供自定义 `database/sql` Driver，SQL 通过 gRPC 代理到核心连接池
6. **共享 Redis**: SDK 提供 Redis 客户端代理，操作通过 gRPC 代理到核心 Redis
7. **热拔插**: 运行时启用/禁用插件，无需重启核心
8. **热路径性能**: 定价/映射数据写入 Redis（插件侧），核心网关直读 Redis（无 gRPC 开销）

---

## 三、整体架构

```
┌─────────────────────────────────────────────────────┐
│                   Core 进程                          │
│                                                     │
│  ┌──────────┐  ┌──────────┐  ┌───────────────────┐ │
│  │PluginRouter│ │ Gin路由  │  │ 插件管理器        │ │
│  │(http.Handler│ │ 认证中间件│  │ - 进程生命周期    │ │
│  │ 包装Gin)   │ │ JWT/APIKey│  │ - 路由代理        │ │
│  └──────────┘  └──────────┘  │ - 前端bundle托管  │ │
│                               └───────────────────┘ │
│                                                     │
│  ┌──────────────── SDK gRPC Server ────────────────┐│
│  │  SQLProxy        RedisProxy       EventBus      ││
│  │  (持有PG连接池)  (持有Redis连接)  (发布订阅)    ││
│  └──────────────────────────────────────────────────┘│
│                                                     │
│  ┌──────── ChannelCacheReader (新增) ──────────────┐│
│  │  从 Redis 直接读取渠道定价/映射数据              ││
│  │  零 gRPC 开销，亚毫秒延迟                        ││
│  └──────────────────────────────────────────────────┘│
└───────────────────────┬─────────────────────────────┘
                        │ gRPC (localhost)
┌───────────────────────┴─────────────────────────────┐
│              插件进程 (channel-management)            │
│                                                     │
│  ┌─────────┐  ┌─────────┐  ┌─────────────────────┐ │
│  │ Handler │  │ Service │  │ CacheWriter          │ │
│  │ (HTTP)  │  │ (CRUD)  │  │ (写Redis供网关读)    │ │
│  └─────────┘  └─────────┘  └─────────────────────┘ │
│                                                     │
│  SDK Client:                                        │
│  - DB: 自定义sql.Driver → gRPC → 核心PG池            │
│  - Redis: 代理客户端 → gRPC → 核心Redis              │
└─────────────────────────────────────────────────────┘
```

---

## 四、已完成的工作

### Phase 1: 插件系统基础设施 ✅

#### 4.1 plugin-sdk/ (独立 Go module)

| 文件 | 功能 |
|------|------|
| `proto/sdk.proto` | CoreSDK gRPC 定义（SQLProxy, RedisProxy, EventBus）|
| `proto/plugin.proto` | PluginLifecycle gRPC 定义（Init, GetManifest, HealthCheck, Shutdown, GetFrontendBundle）|
| `proto/pluginsdk/*.pb.go` | 生成的 gRPC 代码 |
| `plugin.go` | `Plugin` 接口 + 可选接口 `HealthChecker` / `HTTPRegistrar` / `HTTPMux` / `FrontendBundleProvider` |
| `manifest.go` | Manifest / EndpointDecl / FrontendManifest / MenuItemDecl 结构体 |
| `context.go` | `PluginContext` 接口（DB() *sql.DB, Redis() RedisClient, Logger(), Config()）|
| `runner.go` | `Run(p Plugin)` 入口 — 解析 flags、绑定 listener、stdout 握手 JSON、PluginLifecycle gRPC 服务端、SIGINT/SIGTERM 处理、优雅关闭 |
| `driver/sql_driver.go` | 自定义 `database/sql/driver.Driver`，通过 gRPC SQLProxy 代理 SQL。实现 ConnBeginTx/QueryerContext/ExecerContext/Pinger 等全部接口。驱动名 `"plugin-grpc"` |
| `driver/redis_client.go` | Redis 客户端，通过 gRPC RedisProxy 代理 Get/Set/SetEx/Del/HGet/HSet/HGetAll/HDel/Publish/Subscribe |

**go.mod**: `module github.com/Wei-Shaw/sub2api/plugin-sdk`, go 1.26.2

#### 4.2 backend/internal/plugin/ (核心插件宿主, 13 文件)

| 文件 | 行数 | 功能 |
|------|------|------|
| `state.go` | ~60 | PluginState 枚举（Registered/Starting/Running/Errored/Restarting）+ 状态转换守卫 |
| `route_table.go` | ~140 | 不可变 RouteTable，AddPlugin/RemovePlugin 返回新表，Match 最长前缀匹配 |
| `router_middleware.go` | ~190 | PluginRouter 实现 http.Handler，atomic.Pointer 原子换表，复用 JWT/Admin/APIKey 三种鉴权中间件，反向代理到插件 HTTP |
| `restart.go` | ~85 | RestartPolicy 指数退避（1s→300s cap，10 次放弃，稳定 10 分钟归零）|
| `instance.go` | ~110 | PluginInstance 每插件状态 + PluginInfo 快照结构体 |
| `discovery.go` | ~65 | DiscoverPlugins 扫描 `{pluginsDir}/<name>/<name>[.exe]` |
| `repository.go` | ~230 | 裸 SQL CRUD 操作 `plugins` 表，JSONB 配置序列化，EnsureSchema 兜底建表 |
| `migrations.go` | ~170 | RunPluginMigrations，fnv64 计算 per-plugin advisory lock ID，复用核心迁移模式 |
| `health.go` | ~75 | HealthMonitor 周期 gRPC 健康检查，连续 N 次失败触发 onFail 回调 |
| `grpc_server.go` | ~580 | SDKServer 实现 SQLProxy + RedisProxy + EventBus，30s 事务自动 rollback（每 10s 清理），UUID txID |
| `manager.go` | ~700 | PluginManager 协调器：Start/ShutdownAll/Enable/Disable/Restart/List/Get + BindRouter + GetPluginManifestsJSON + UpdateConfig + 进程启动握手 + 健康监控 + 自动重启调度 |
| `config.go` | ~80 | 本地 Config 结构体 + withDefaults |
| `wire.go` | ~12 | Wire ProviderSet |

#### 4.3 核心集成改动

| 文件 | 改动 |
|------|------|
| `backend/internal/server/http.go` | 新增 `ProvidePluginRouter()`，`ProvideHTTPServer` 参数加 `*plugin.PluginRouter`，Handler 链: `h2c(MaxBytes(PluginRouter(gin.Engine)))` |
| `backend/internal/server/router.go` | `SetupRouter` 参数加 `*plugin.PluginManager`，在 frontendServer 上调用 `SetPluginManifestProvider(pluginManager)` |
| `backend/internal/web/embed_on.go` | 新增 `PluginManifestProvider` 接口 + `SetPluginManifestProvider` 方法，`injectSettings()` 同时注入 `window.__PLUGIN_MANIFESTS__` |
| `backend/internal/web/embed_off.go` | 新增 stub 接口和 no-op 方法（非 embed 构建）|
| `backend/internal/config/config.go` | 新增 `PluginConfig` / `PluginRestartConfig` 结构体，`Config.Plugins` 字段，`setDefaults()` 中 8 个默认值 |
| `backend/internal/handler/admin/plugin_handler.go` | Admin API handler（List/Get/Enable/Disable/Restart/UpdateConfig），PluginManager 接口，nil 时返回 503 |
| `backend/internal/server/routes/admin.go` | 注册 `/api/v1/admin/plugins` 路由组（6 个端点）|
| `backend/internal/handler/handler.go` | AdminHandlers 新增 `Plugin *admin.PluginHandler` |
| `backend/internal/handler/wire.go` | `ProvidePluginHandler(manager *plugin.PluginManager)` |
| `backend/cmd/server/wire.go` | Application 新增 `PluginManager` 字段，`providePluginConfig`/`providePluginManager`，cleanup 新增 PluginManager step |
| `backend/cmd/server/wire_gen.go` | 手动同步：pluginManager 创建 → pluginHandler 注入 → BindRouter → cleanup |
| `backend/cmd/server/main.go` | `app.PluginManager.Start(ctx)` 启动插件系统（30s 超时，失败仅 log warning）|
| `backend/migrations/101_create_plugins.sql` | `plugins` 表 + `plugin_migrations` 表 |

#### 4.4 前端插件系统

| 文件 | 功能 |
|------|------|
| `frontend/src/plugins/loader.ts` | 读取 `window.__PLUGIN_MANIFESTS__`，`getPluginMenuItems(section)`，`registerPluginRoutes(router)` |
| `frontend/src/views/plugin/PluginView.vue` | 插件页面占位容器（显示插件名 + 占位提示）|
| `frontend/src/router/index.ts` | 调用 `registerPluginRoutes(router)` |
| `frontend/src/components/layout/AppSidebar.vue` | 合并 pluginAdminNavItems / pluginUserNavItems 到侧边栏 |

#### 4.5 hello-world 测试插件

| 文件 | 功能 |
|------|------|
| `plugins/hello-world/main.go` | 完整测试插件：`/hello`（无 auth）、`/db-test`（admin auth, SELECT 1）、`/redis-test`（admin auth, SetEx+Get）|
| `plugins/hello-world/go.mod` | 独立 module，replace 指向 `../../plugin-sdk` |

#### 4.6 热拔插生命周期

**插件进程启动协议**:
1. Core 用 `exec.CommandContext(binary, "--core-sdk-addr", sdkAddr)` 启动插件
2. 插件绑定 gRPC 和 HTTP 到 `:0`（随机端口）
3. 插件向 stdout 写一行 JSON: `{"protocol":1,"grpc_addr":"127.0.0.1:50123","http_addr":"127.0.0.1:50124"}`
4. Core 读取（10s 超时），dial gRPC，调 GetManifest/Init
5. Core 注册路由到 RouteTable（原子替换），启动健康监控
6. 状态 → Running

**状态机**: Registered → Starting → Running/Errored → Restarting → Starting...

**循环依赖解决**: `PluginManager.BindRouter(router)` 后绑定模式。创建顺序: PluginManager(nil router) → handlers → gin.Engine → PluginRouter → BindRouter

**默认行为**: `plugins.enabled=false`（默认）时，PluginManager 为 nil，所有插件 API 返回 503，完全向后兼容。

---

### Phase 2: 渠道管理插件抽取 (部分完成)

#### 4.7 渠道管理插件后端 ✅

`plugins/channel-management/` 独立 Go module:

| 文件 | 功能 |
|------|------|
| `main.go` | `pluginsdk.Run(&ChannelPlugin{})` |
| `plugin.go` | Manifest 声明 + Init（接入 DB/Redis/CacheWriter）+ Shutdown + RegisterHTTP |
| `service/channel.go` | Channel/ChannelModelPricing/PricingInterval 等类型定义 |
| `service/channel_service.go` | ChannelService CRUD + 缓存 + CacheWriter 集成（Create/Update/Delete 后自动写 Redis）|
| `service/cache_writer.go` | **CacheWriter**：按 GATEWAY_CACHE_SPEC 将定价/映射数据写入 Redis，供核心网关直读 |
| `repository/channel_repo.go` | 数据访问（裸 SQL），通过 SDK 的 gRPC 代理 `*sql.DB` |
| `repository/channel_repo_pricing.go` | 模型定价 CRUD |
| `handler/channel_handler.go` | Admin API handler（所有渠道 CRUD 端点）|
| `internal/errors/errors.go` | ApplicationError |
| `internal/pagination/pagination.go` | 分页参数 |
| `internal/response/response.go` | 统一响应格式 |

#### 4.8 渠道管理插件前端 ✅

`plugins/channel-management/frontend/`:

| 文件 | 功能 |
|------|------|
| `package.json` | @sub2api/plugin-channel-management，peerDeps: vue/vue-router/vue-i18n/pinia/axios |
| `vite.config.ts` | Vite library mode，ES 输出 |
| `tsconfig.json` | 严格模式 |
| `src/index.ts` | 导出 ChannelsView + setClient + i18n + route |
| `src/views/ChannelsView.vue` | 渠道管理主视图（1085 行）|
| `src/components/` | IntervalRow.vue, ModelTagInput.vue, PricingEntryCard.vue, types.ts |
| `src/api/channels.ts` | 渠道 API 客户端（路径: `/plugin/channel-management/admin/channels`）|
| `src/api/client.ts` | setClient/getClient（接收 host axios 实例）|
| `src/i18n/en.ts` + `zh.ts` | 渠道相关 i18n keys |

#### 4.9 网关解耦设计 ✅

| 文件 | 功能 |
|------|------|
| `plugins/channel-management/GATEWAY_CACHE_SPEC.md` | **348 行**完整 Redis 缓存契约：K1-K6 + P1 键格式、JSON schema、TTL 15 分钟、失效触发表、安全降级策略 |
| `backend/internal/service/channel_cache_reader.go` | **414 行**核心侧 Redis 读取器：GetChannelMeta/ResolveChannelMapping/IsModelRestricted/GetChannelModelPricing，仅依赖 `*redis.Client`，失败返回安全零值 |
| `plugins/channel-management/GATEWAY_MIGRATION_GUIDE.md` | 网关迁移指南：gateway_service.go 7 个调用点 + openai_gateway_service.go 7 个调用点的逐个替换方案 |

#### 4.10 核心渠道代码清理 ✅

**已备份为 .bak 的文件**（13 个）:
```
backend/internal/service/channel_service.go.bak          (942 行)
backend/internal/service/channel_service_test.go.bak      (2405 行)
backend/internal/service/channel_test.go.bak              (435 行)
backend/internal/service/gateway_channel_restriction_test.go.bak
backend/internal/service/gateway_channel_restriction_fallback_test.go.bak
backend/internal/service/openai_channel_restriction_test.go.bak
backend/internal/service/model_pricing_resolver_test.go.bak
backend/internal/repository/channel_repo.go.bak           (486 行)
backend/internal/repository/channel_repo_pricing.go.bak   (291 行)
backend/internal/repository/channel_repo_test.go.bak
backend/internal/handler/admin/channel_handler.go.bak     (398 行)
backend/internal/handler/admin/channel_handler_test.go.bak
backend/internal/handler/admin/account_handler_mixed_channel_test.go.bak
```

**保留的文件**:
- `backend/internal/service/channel.go` — **类型文件**（Channel, ChannelModelPricing, BillingMode 等），网关仍需要这些类型。增加了空存根 `ChannelService struct{}` + 5 个零值返回方法
- `backend/internal/service/channel_cache_reader.go` — 网关将使用的新 Redis 读取器

**Wire/路由清理**:
- `service/wire.go` 移除了 `NewChannelService`
- `repository/wire.go` 移除了 `NewChannelRepository`
- `handler/wire.go` 移除了 `channelHandler` 参数和 `admin.NewChannelHandler`
- `handler/handler.go` 移除了 `Channel` 字段
- `routes/admin.go` 移除了 `registerChannelRoutes` 函数和调用
- `wire_gen.go` 中 `channelService := &service.ChannelService{}` 空存根

---

## 五、当前状态

### 编译状态（全部通过）
- `backend/go build ./...` ✅
- `plugin-sdk/go build ./...` ✅
- `plugins/channel-management/go build ./...` ✅
- `plugins/hello-world/go build ./...` ✅
- `backend/go vet ./...` ✅

### 运行时状态
- **核心渠道功能已降级**：GatewayService 中 `channelService` 是空存根 `&service.ChannelService{}`，所有方法返回零值
- **插件系统默认关闭**：`plugins.enabled=false`（默认），PluginManager 为 nil
- **channel_cache_reader.go 已创建但未接入**：GatewayService 仍使用空存根而非 CacheReader

### 关键 go.mod 依赖关系
```
backend/go.mod:
  require github.com/Wei-Shaw/sub2api/plugin-sdk v0.0.0
  replace github.com/Wei-Shaw/sub2api/plugin-sdk => ../plugin-sdk

plugins/channel-management/go.mod:
  require github.com/Wei-Shaw/sub2api/plugin-sdk v0.0.0
  replace github.com/Wei-Shaw/sub2api/plugin-sdk => ../../plugin-sdk

plugins/hello-world/go.mod:
  require github.com/Wei-Shaw/sub2api/plugin-sdk v0.0.0
  replace github.com/Wei-Shaw/sub2api/plugin-sdk => ../../plugin-sdk
```

---

## 六、待完成的工作

### 6.1 GatewayService 切换到 ChannelCacheReader（最关键的剩余任务）

**目标**: 将 GatewayService 中的 `channelService` 替换为 `channelCacheReader`，使网关从 Redis 读取渠道数据（由插件写入），而非调用已移除的 ChannelService。

**详细指南**: `plugins/channel-management/GATEWAY_MIGRATION_GUIDE.md`

**具体步骤**:

1. **在 GatewayService 结构体中**:
   - 添加字段 `channelCacheReader *ChannelCacheReader`
   - 保留 `channelService` 字段暂时不删（存根），后续可移除

2. **替换调用点**（gateway_service.go，约 7 处）:
   | 旧调用 | 新调用 | 行号区间 |
   |--------|--------|----------|
   | `s.channelService.ResolveChannelMapping(groupID, model)` | `s.channelCacheReader.ResolveChannelMapping(ctx, groupID, platform, model)` | ~8082 |
   | `s.channelService.IsModelRestricted(groupID, model)` | `s.channelCacheReader.IsModelRestricted(ctx, groupID, platform, model)` | ~8095 |
   | `s.channelService.ResolveChannelMappingAndRestrict(...)` | 组合 ResolveChannelMapping + IsModelRestricted | ~8102 |
   | `s.checkChannelPricingRestriction(...)` | 使用 CacheReader 版本 | ~1180, 1237 |
   | `s.resolveChannelPricing(...)` | `s.channelCacheReader.GetChannelModelPricing(ctx, groupID, platform, model)` | ~7884 |
   | `s.channelService.GetChannelForGroup(groupID)` | `s.channelCacheReader.GetChannelMeta(ctx, groupID, platform)` | 多处 |

3. **替换 openai_gateway_service.go 调用点**（约 7 处，同上模式）

4. **替换 model_pricing_resolver.go**:
   - `channelService.GetChannelModelPricing` → `channelCacheReader.GetChannelModelPricing`

5. **Wire 接入**:
   - `NewChannelCacheReader(redisClient)` 添加到 wire
   - 注入到 GatewayService 构造函数

6. **关键注意**: 所有新调用需要额外的 `platform` 参数（从 `apiKey.Group.Platform` 获取），因为 Redis 键按 `{groupID}:{platform}` 分区

### 6.2 端到端验证

1. 配置 `config.yaml` 启用插件: `plugins.enabled: true, dir: "data/plugins"`
2. 将 hello-world 二进制放入 `data/plugins/hello-world/`
3. 启动核心，调用 `POST /api/v1/admin/plugins/hello-world/enable`
4. 验证: `GET /api/v1/plugin/hello-world/hello` 返回正确响应
5. 验证: DB test 和 Redis test 端点正常
6. 验证: 禁用/重启/自动重启功能

### 6.3 可选后续工作

- **前端 Vite library build**: `plugins/channel-management/frontend/` 需要 `pnpm install && pnpm build`
- **前端 bundle 加载**: 核心 `GetFrontendBundle` gRPC 流式传输插件前端资源
- **测试覆盖**: plugin manager / cache writer / cache reader 的单元测试
- **channel.go 存根清理**: 当 GatewayService 完全切换到 CacheReader 后，可删除 channel.go 中的存根方法
- **数据库迁移**: 核心的 channels 相关迁移（081-095）保留不删，插件用 `IF NOT EXISTS`

---

## 七、Redis 缓存键规范摘要

（完整规范见 `plugins/channel-management/GATEWAY_CACHE_SPEC.md`）

| 键 | 格式 | 用途 |
|----|------|------|
| K1 | `plugin:channel:meta:{groupID}:{platform}` | 渠道元数据（名称、restrict_models、billing_model_source）|
| K2 | `plugin:channel:pricing:{groupID}:{platform}:{model}` | 精确模型定价 |
| K3 | `plugin:channel:pricing:wildcard:{groupID}:{platform}` | 通配符定价信封（JSON 数组）|
| K4 | `plugin:channel:mapping:{groupID}:{platform}:{model}` | 精确模型映射（纯字符串）|
| K5 | `plugin:channel:mapping:wildcard:{groupID}:{platform}` | 通配符映射信封（JSON 数组）|
| K6 | `plugin:channel:all-groups` | 全部已激活 groupID 列表 |
| P1 | `plugin:channel:invalidate` Pub/Sub | 缓存失效通知（可选 v2）|

TTL: 15 分钟。写侧（插件 CacheWriter）在 CRUD 后立即重建。读侧（核心 CacheReader）cache miss 时返回安全零值。

---

## 八、关键文件索引

### 必读文件（理解架构）
- `plugin-sdk/plugin.go` — Plugin 接口定义
- `plugin-sdk/runner.go` — 插件启动流程
- `backend/internal/plugin/manager.go` — 核心插件管理器
- `backend/internal/plugin/router_middleware.go` — 动态路由
- `backend/internal/service/channel_cache_reader.go` — 网关侧 Redis 读取器
- `plugins/channel-management/service/cache_writer.go` — 插件侧 Redis 写入器
- `plugins/channel-management/GATEWAY_MIGRATION_GUIDE.md` — **网关切换的具体操作指南**
- `plugins/channel-management/GATEWAY_CACHE_SPEC.md` — Redis 缓存契约

### 需要修改的文件（完成 6.1 任务）
- `backend/internal/service/gateway_service.go` — 主网关服务（~8000 行）
- `backend/internal/service/openai_gateway_service.go` — OpenAI 网关
- `backend/internal/service/model_pricing_resolver.go` — 定价解析
- `backend/internal/service/wire.go` — 添加 CacheReader provider
- `backend/cmd/server/wire_gen.go` — 手动更新 DI（wire 命令因预存问题无法运行）

### 注意事项
- `wire` 命令在本项目中**无法正常运行**（缺少 `[]time.Duration` provider，预存问题），所有 `wire_gen.go` 改动必须**手动编辑**
- `backend/go.mod` 有 `replace github.com/Wei-Shaw/sub2api/plugin-sdk => ../plugin-sdk` 本地联调指令
- `.bak` 文件不会被 Go 编译器识别，保留作为参考
