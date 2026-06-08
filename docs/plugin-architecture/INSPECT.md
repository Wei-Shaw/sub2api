# 插件系统边界诊断报告

> Inspector 产出，时间 2026-04-26，基线 commit `26a9eab6` (feat/plugin-system-fixes)

## 1. 插件代码独立性 ✅

- `plugins/channel-management/` 是独立 Go module：`plugins/channel-management/go.mod:1` `module github.com/Wei-Shaw/sub2api/plugins/channel-management`，未声明对 `github.com/Wei-Shaw/sub2api/backend` 的 require。
- `plugins/hello-world/go.mod:1` 同上，独立 module。
- 两者均仅通过 `replace github.com/Wei-Shaw/sub2api/plugin-sdk => ../../plugin-sdk` 依赖 SDK：`plugins/channel-management/go.mod:45`、`plugins/hello-world/go.mod:16`。
- 前端独立 npm package：`plugins/channel-management/frontend/package.json:2` `"name": "@sub2api/plugin-channel-management"`，private+独立 vite 配置。

## 2. Core → Plugin 依赖反转检查 ✅

- `Grep "github.com/Wei-Shaw/sub2api/plugins/" backend/` → **No matches found**（0 行）。
- `Grep "Wei-Shaw/sub2api/plugins/" backend/` → 0 行。
- `backend/cmd/server/wire_gen.go` / `wire.go` 中无任何 `plugins/...` import；`PluginManager` 通过 gRPC + 二进制路径注入，类型层完全解耦。
- 前端：`Grep "from '@sub2api/plugin-" frontend/src` → 0 行；`Grep "from '../../../plugins/" frontend/src` → 0 行；`Grep "from '@/plugins/" frontend/src` 命中 4 处但全部指向 host 自身的 `frontend/src/plugins/{loader,loader-runtime,sdk}`（loader 模块），不是 plugin 源码。证据：`frontend/src/main.ts:7`、`frontend/src/router/index.ts:12`、`frontend/src/views/plugin/PluginView.vue:48-49`、`frontend/src/components/layout/AppSidebar.vue:190`。

## 3. 构建产物与 Dockerfile ⚠️ 部分缺失

Dockerfile 多阶段（`Dockerfile:1-187`）：
- Stage 1 `frontend-builder`（19-32）：只 build host frontend，**不**涉及 plugin frontend。
- Stage 2 `backend-builder`（37-82）：build core 二进制，`-tags embed` 把 host frontend dist 嵌入。
- **Stage 2.5 `plugin-builder`（91-120）**：遍历 `/src/plugins/*/`，每个有 `go.mod` 的目录执行 `go build ... -o /out/plugins/<name>/<name> ./`。仅构建 Go 二进制。
- Stage 4 final（130-187）：`COPY --from=plugin-builder /out/plugins /app/plugins`（169 行），与 BuiltinDir 默认值一致。

**缺失**：
- plugin-builder 中 `Grep "pnpm build|npm run build"` → 0 行。即 plugin frontend bundle **不会**在 Docker 构建时产生。
- frontend stage 也没有 `COPY plugins/*/frontend/dist`。
- 结果：channel-management 即使后端 plugin 二进制成功部署，前端 bundle 缺失（且其 `plugin.go` 也未声明 `EntryJS`，参见第 6 节）。

## 4. 运行时加载链路

启动时序：
1. `backend/cmd/server/main.go:148` 调 `initializeApplication` → wire 树创建 `PluginManager`（`wire_gen.go:274`），构造时注入 `plugin.Config{BuiltinDir, PluginsDir, AutoEnableBuiltin}`。配置翻译在 `wire_gen.go:36-58`。
2. `main.go:155-161` 调 `app.PluginManager.Start(ctx)`（30s timeout）。
3. `manager.go:72 Start` → `EnsureSchema` → `startSDKServer`（启动 SDK gRPC 服务）→ `syncFromDisk(ctx)`（90 行）。
4. `syncFromDisk`（`manager.go:151-181`）调 `DiscoverFromDirs(BuiltinDir, PluginsDir)`（`discovery.go:21`）；遇到 DB 中已有记录则跳过；新插件按 `enabled := d.Builtin && AutoEnableBuiltin`（`manager.go:166`）写入。
5. `Start` 循环 `repo.List` → 对每个 `rec.Enabled=true` 调 `EnablePlugin`（`manager.go:96-110`）。
6. `EnablePlugin` → `binaryPathFor`（`manager.go:596-608`，BuiltinDir 优先）→ `spawnAndConnect`（`manager.go:618`）：`exec.CommandContext(procCtx, inst.BinaryPath)` → 读 stdout handshake JSON → gRPC dial → `GetManifest` → `Init` (含 capabilities) → `buildRouteEntries` → `router.SwapRouteTable` → 启动健康监控 goroutine。
7. 二进制路径来自 `m.cfg.BuiltinDir + "/" + name + "/" + pluginBinaryName(name)`，linux 是 `<name>`，windows 是 `<name>.exe`（`discovery.go:108-114`）。

## 5. 内置 vs 非内置 + 默认启用 ✅

- 标记规则：`DiscoverFromDirs` 先扫 BuiltinDir，给每条加 `Builtin=true`；再扫 user dir，标 `false`。同名 builtin 覆盖（`discovery.go:25-46`）。
- 默认启用：`syncFromDisk` 仅在 DB 无记录（`ErrPluginNotFound`）时插入新行，`enabled := d.Builtin && m.cfg.AutoEnableBuiltin`（`manager.go:166`）。非 Builtin → 永远 `enabled=false`。
- 用户 disable 后启动不会被覆盖：`manager.go:158-160` 若 `repo.Get` 成功（已存在记录）即 `continue`，不修改 enabled 字段。
- 默认值：`backend/internal/config/config.go:1382` `viper.SetDefault("plugins.builtin_dir", "/app/plugins")`、`config.go:1384` `viper.SetDefault("plugins.auto_enable_builtin", true)`。`PluginsDir` 默认空（须 yaml 显式给出 `plugins.dir`）。
- 注意：`plugin.DefaultConfig()`（`config.go:49-62`）本身没填 `BuiltinDir/AutoEnableBuiltin`，但 `providePluginConfig`（`wire_gen.go:36-58`）会从 viper 读到的 `cfg.Plugins.*` 写进去，所以最终运行时是 `/app/plugins` + true。

## 6. channel-management 插件当前完整度

- **后端：✅** `plugins/channel-management/plugin.go:31-182` 实现 Manifest/Init/Shutdown/RegisterHTTP/HealthCheck，路由、Redis cache writer、capability 协商完整。
- **前端 manifest 字段：❌** `plugin.go:63-86` `Frontend` 仅声明 `MenuItems` 与 `I18nNamespaces`，**没有 EntryJS / EntryCSS / Routes**。`Grep "EntryJS|EntryCSS" plugins/channel-management/plugin.go` → 0 行。这意味着前端 SDK loader 没有可加载的 bundle，菜单点击仍依赖 host 的路由。
- **前端 dist 产物：❌** `plugins/channel-management/frontend/dist/` 不存在。`package.json:18-21` 有 `build` 脚本（`vue-tsc -b && vite build`），但当前未跑过；Dockerfile 也未触发它（见第 3 节）。
- **FrontendBundleProvider 实现：❌** `Grep "OpenFrontendFile|FrontendBundleProvider" plugins/channel-management` → 0 行。对照 `plugins/hello-world/main.go:131-142` 已实现 OpenFrontendFile + `//go:embed all:frontend/dist`。
- **host 端待迁移文件清单**（host 仍是事实上的 channels 实现，下游 Implementer 要把它搬走或彻底改为代理到 plugin）：
  - `frontend/src/views/admin/ChannelsView.vue`（host 当前路由组件）
  - `frontend/src/api/admin/channels.ts`
  - `frontend/src/components/admin/channel/IntervalRow.vue`
  - `frontend/src/components/admin/channel/ModelTagInput.vue`
  - `frontend/src/components/admin/channel/PricingEntryCard.vue`
  - `frontend/src/components/admin/channel/types.ts`
  - `frontend/src/router/index.ts:339-350`（`AdminChannels` 路由仍指向 `@/views/admin/ChannelsView.vue`）
  - 注意：plugin 内已存在等价副本 `plugins/channel-management/frontend/src/views/ChannelsView.vue` + `components/{IntervalRow,ModelTagInput,PricingEntryCard}.vue` + `api/channels.ts`（已经搬过一遍但 host 副本未删除，二者并存）。

## 7. 红线违反清单（必须修）

按严重程度排序：

1. **【高】channel-management 前端未真正脱离 core**：host `ChannelsView.vue` 等 6 个文件仍在 `frontend/src/`，`router/index.ts:340-349` 仍直接 import host 路径，相当于"复制了一份到 plugin/ 但 host 仍然是真实运行的版本"。违反"渠道管理是内置插件、不嵌入 core"。
2. **【高】channel-management plugin Manifest 未声明 EntryJS**：plugin.go:63-86 只有 MenuItems/I18nNamespaces，前端 loader（`frontend/src/plugins/loader-runtime.ts`）拿不到 bundle URL，无法动态加载。
3. **【高】Dockerfile plugin-builder stage 不构建 plugin frontend**：Dockerfile:91-120 只 `go build`，没有 `pnpm install && pnpm build`，也没有 `COPY plugins/*/frontend/dist`。即使 plugin 声明了 EntryJS，运行时也读不到 dist 文件。
4. **【中】channel-management 未实现 FrontendBundleProvider.OpenFrontendFile**：核心通过 GetFrontendBundle gRPC 拉 bundle 的链路无法工作，对照 hello-world 实现缺失。
5. **【低】host 与 plugin 内 i18n key 重复 (`admin.channels.*`)**：host `frontend/src/i18n/locales/en.ts` 没有 `admin.channels` 命名空间（grep 0 行），但 ChannelsView.vue 引用了；plugin 内 `i18n/{en,zh}` 是真实定义。说明 host ChannelsView 现在依赖 plugin 注入的 i18n namespace `channel-management`，运行时表现待 Implementer 验证。

## 8. 已合规项（确认保留）

- 插件独立 Go module + replace 指向 SDK：✅ 不要回退到把 plugin 拉进 backend module。
- core → plugin 零反向依赖（go + ts 双侧 grep 证实）：✅
- `BuiltinDir=/app/plugins`、`AutoEnableBuiltin=true` 默认值通过 viper 设置（`config.go:1382-1384`）：✅
- Dockerfile plugin-builder stage 已存在且产出位置正确（`/out/plugins` → `/app/plugins`）：✅ 仅需补 frontend 步骤，不要重构骨架。
- discovery + syncFromDisk 的"DB 已存在则不覆盖 enabled"语义：✅ 用户手动 disable 不会被覆盖。
- hello-world 是合格参考样板（`go:embed` + EntryJS + OpenFrontendFile）：✅ Implementer 应直接照搬到 channel-management。
- frontend 端有 host SDK 注入 + 动态 loader（`frontend/src/main.ts:7`、`frontend/src/plugins/loader-runtime.ts`、`PluginView.vue:48-49`）：✅ 框架已就绪，只缺 channel-management 实际供货。
