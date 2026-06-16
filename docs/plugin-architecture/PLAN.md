# PLAN.md — channel-management 插件迁移计划

> Planner 产出，基线：commit `f722f36d` (`feat/plugin-system-fixes`)，上游：`INSPECT.md`。
> 下游：Designer → Implementer。本文档只规划"做什么 + 为什么 + 怎么验"，不含实现细节。

## 0. 关键发现（来自 Planner，Inspector 未提及）

`plugins/channel-management/frontend/src/index.ts` 当前是 **named exports** 形态（host source-tree 旧 import 模式），与 `loader-runtime.ts` 期望的 `default export { install(sdk) }` runtime 协议**不兼容**。所以 plugin 内已有的"副本"不能直接用，必须改造 entry 协议（见 T3）。

## 1. 目标陈述

迁移完成后达到以下"成功状态"：

1. core 二进制（`backend/cmd/server`）经 strings/grep 检查不再含 `ChannelsView` / `frontend/src/views/admin/ChannelsView.vue` 等 host 渠道实现，host frontend bundle 中也搜不到 `admin/channels` 路由 import。
2. `plugins/channel-management` 后端 + 前端 bundle 均由 Dockerfile 构建并落地到运行镜像 `/app/plugins/channel-management/{channel-management,frontend/dist/entry.js}`。
3. plugin Manifest 通过 SDK 上报 `EntryJS=dist/entry.js` + `Routes` + `I18nNamespaces=["channel-management"]`；插件 `OpenFrontendFile` 通过 `go:embed` 读取 dist。
4. 浏览器访问 `/admin/channels` 时，加载的 JS bundle URL 形如 `/api/v1/plugin-assets/channel-management/dist/entry.js`，渠道增删改查行为与迁移前等价。
5. 关闭该插件后，菜单与路由消失，host 侧不残留任何渠道功能入口。

## 2. 任务分解（按依赖顺序）

### T1 — diff 现有 plugin 副本与 host 副本，产出迁移清单
- 目标：明确 plugin 内已存在的 6 个副本与 host 6 个文件之间的内容差异，避免后续误删 host 中独有的修补。
- Inputs：`plugins/channel-management/frontend/src/{views,components,api,i18n,index.ts}`、`frontend/src/{views/admin/ChannelsView.vue, components/admin/channel/*, api/admin/channels.ts, router/index.ts L339-350}`、`INSPECT.md` §6。
- Outputs：本任务不改代码，仅在 DESIGN.md / PR 描述附 "host-only 改动需补 patch 的清单"（文本）。
- 验收：清单逐项标注 "已等价" / "plugin 缺失需补" / "host 独有需丢弃"；至少覆盖 7 个文件。
- 依赖：无。

### T2 — 补齐 plugin frontend 缺失依赖与样式 / 工具
- 目标：让 plugin frontend 在不 import host `@/` 别名的前提下能独立 build；新增 plugin 端 stub（如 useTheme 来自 hostSdk、notify 来自 hostSdk）。
- Inputs：T1 清单、`frontend/src/plugins/sdk/host-sdk*`、`plugins/hello-world/frontend/dist/entry.js` 样板。
- Outputs：`plugins/channel-management/frontend/src/**`（除 index.ts、vite.config.ts、package.json 外的源文件改动）。
- 验收：`pnpm --filter @sub2api/plugin-channel-management typecheck` 通过；该目录内 `grep -R "from '@/"` 命中 0 行。
- 依赖：T1。

### T3 — 改造 plugin frontend 为 runtime entry 协议（install(sdk)）
- 目标：把 `src/index.ts` 从 host source-tree 的 named-export 模式改为 hello-world 同款 runtime entry：default export `{ install(sdk) }`，返回 `{ components: { 'ChannelsView.vue': ... } }`，并在 install 时注入 i18n。vite.config 改为产出 `dist/entry.js`，外部化所有 vue 系。
- Inputs：`plugins/hello-world/frontend/dist/entry.js`、`frontend/src/plugins/loader-runtime.ts` install 协议、Designer 对"是否单文件 / 如何处理样式"的决议（见 §6 Q1/Q2）。
- Outputs：`plugins/channel-management/frontend/{src/index.ts, vite.config.ts, package.json}`、新增 `plugins/channel-management/frontend/dist/entry.js`（可由本地一次性 build 产出，但生产以 Docker stage 为准）。
- 验收：`pnpm --filter @sub2api/plugin-channel-management build` 在 `dist/` 产出 `entry.js`；本地 `node -e "import('./dist/entry.js').then(m => console.log(typeof m.default.install))"` 输出 `function`。
- 依赖：T2。

### T4 — plugin Manifest 声明前端契约 + 实现 OpenFrontendFile
- 目标：`plugin.go` `FrontendManifest` 增 `EntryJS:"dist/entry.js"` 与 `Routes:[{Path:"/admin/channels", Name:"AdminChannels", ComponentPath:"ChannelsView.vue"}]`；新增 `frontendAssets embed.FS` 与 `OpenFrontendFile`；MenuItem `Path` 与 Route 对齐。
- Inputs：`plugins/hello-world/main.go` 参考；INSPECT.md §7 红线 2 与 4。
- Outputs：`plugins/channel-management/plugin.go`、可能新增 `plugins/channel-management/embed.go`。
- 验收：`go build ./...` 在 `plugins/channel-management/` 通过；插件二进制启动后 `GetManifest` gRPC 返回的 JSON 中含 `frontend.entry_js="dist/entry.js"` 和 1 条 route。
- 依赖：T3（embed 路径必须有 dist 才能 build）。

### T5 — Dockerfile plugin-builder stage 增加 frontend 构建步骤
- 目标：`plugin-builder` stage（或新增 `plugin-frontend-builder` stage）遍历 `plugins/*/frontend/`，存在 `package.json` 时跑 `pnpm install --frozen-lockfile && pnpm build`；保证 `embed.FS` 在 `go build` 之前已就绪。
- Inputs：`Dockerfile:91-120`、`Dockerfile:19-32` (frontend-builder 节点版本) 、Designer 对 "pnpm 还是 npm / 是否单 stage / 是否复用 frontend-builder node_modules cache" 的决策（§6 Q3）。
- Outputs：`Dockerfile`。
- 验收：`docker build .` 后 `docker run --rm <img> ls /app/plugins/channel-management/frontend/dist/entry.js` 成功；同样命令对 `hello-world` 也仍然通过（不要破坏现有产物）。
- 依赖：T3、T4（不严格依赖 T4 但合并发布更顺）。

### T6 — 删除 host 端渠道相关源码与路由
- 目标：移除 INSPECT.md §6 的 7 个 host 路径；`router/index.ts` 不再包含 `AdminChannels` 静态路由（改由 plugin manifest 注入）；i18n keys 视 Designer 决议（§6 Q4）保留或删除。
- Inputs：T1 清单、`frontend/src/router/index.ts:339-350`、所有引用 `@/views/admin/ChannelsView.vue` / `@/api/admin/channels` / `@/components/admin/channel/*` 的入口。
- Outputs：`frontend/src/router/index.ts`、删除文件 7 个、可能调整 `frontend/src/i18n/locales/{en,zh}.ts`。
- 验收：`grep -R "admin/channels" frontend/src` 命中 0 行（除 plugin loader 通用逻辑）；host 前端 `pnpm build` 通过；host 路由表生成结果不含 `AdminChannels` 静态条目。
- 依赖：T4（plugin 必须先能注入路由）+ T5（CI 构建链路就绪）；建议在 T5 通过后立即合并避免长期断流。

### T7 — 端到端联调验证（dev + 镜像两套）
- 目标：dev 模式 (`make dev` 或等价) + Docker 镜像两个环境下，验收清单 §4 全部打勾。
- Inputs：T5、T6 完成后的 main 分支。
- Outputs：本任务不产源码改动；产出验收报告附在 PR 中。
- 验收：见 §4 的 8 条。
- 依赖：T5、T6。

### T8 — 构建产物体积/缓存基线回归（可选）
- 目标：测量 host frontend bundle 是否真的瘦了（删除 ChannelsView 后 chunk gzip 变小）、Docker 多阶段缓存命中是否退化超过 30%。
- Outputs：PR 描述中附 before/after 数据。
- 依赖：T7。

**关键路径**：T1 → T2 → T3 → T4 → T5 → T6 → T7。T8 可并行/省略。T2 与 T1 之间允许同一个 agent 串起来；T5 与 T4 可由不同 agent 并行（T5 只需要 dist 路径约定、不需要 dist 实际内容）。

## 3. 风险点

| # | 风险 | 触发条件 | 缓解 |
|---|------|----------|------|
| R1 | plugin frontend 漏掉 host 独有 commit（如最近 `IconTag` 调整、`PlatformPicker` 修复） | T1 diff 不彻底 | T1 必须列文件级 + 关键 commit 范围；Designer 决定是否 cherry-pick |
| R2 | i18n key 全局 namespace 冲突或缺失 | host 删 `admin.channels.*` keys 但其他视图（如 dashboard）仍引用 | 删除前 grep `t('admin.channels` 全仓；只删未被引用的 |
| R3 | Dockerfile node_modules 缓存膨胀 / 构建时间 > 8 分钟 | plugin-builder 每次重新跑 `pnpm install` | 复用 frontend-builder 的 pnpm store cache，或 mount cache（由 Designer Q3 决） |
| R4 | dist 路径与 OpenFrontendFile 不一致导致 404 | Manifest 写 `dist/entry.js`，embed 写 `frontend/dist`，OpenFrontendFile 拼接错前缀 | 严格照搬 hello-world 的 `frontend/`+rel 拼法；T7 端到端验 URL |
| R5 | 老书签 `/admin/channels` 在路由删除瞬间 404 | T6 合并到 T4/T5 之前合并、或部署顺序错 | 强制 T6 依赖 T4+T5；plugin 必须 builtin+auto-enable，确保路由动态注入永远先于用户访问 |

## 4. 验收清单

- [ ] core `go build ./backend/...` 通过；strings 二进制 `grep -c ChannelsView` = 0
- [ ] plugin `go build` 在 `plugins/channel-management/` 通过，产出 `channel-management` 二进制
- [ ] `plugins/channel-management/frontend/dist/entry.js` 存在且 `default.install` 是函数
- [ ] host `pnpm build` 通过；产物 chunk 不含 `ChannelsView`
- [ ] `docker build .` 通过；镜像内同时存在 `/app/plugins/channel-management/channel-management` 与 `/app/plugins/channel-management/frontend/dist/entry.js`
- [ ] 启动后 `GET /api/v1/admin/plugins` 返回的 channel-management 节点含 `entry_js_url=/api/v1/plugin-assets/channel-management/dist/entry.js`
- [ ] 浏览器 DevTools Network 面板：访问 `/admin/channels` 实际加载的 JS URL 来自 `/api/v1/plugin-assets/channel-management/...`
- [ ] 渠道列表 / 创建 / 编辑 / 删除 / 定价端点功能等价（手动冒烟 5 个动作）
- [ ] `grep -R "from '@/views/admin/ChannelsView'" frontend/src` 命中 0 行
- [ ] 在 admin UI 关闭 channel-management 插件后，刷新页面 `/admin/channels` 返回 PluginView 降级页或 404，且菜单消失
- [ ] hello-world 仍正常工作（回归不破坏）

## 5. 不在范围内（Out of Scope）

- 渠道功能本身的新特性（计费、监控扩展、bulk import）
- `iframe` isolation 的实现（loader-runtime 占位继续抛 not implemented）
- 用户上传插件（非 builtin、`plugins.dir`）的 UI 与签名校验
- 测试覆盖率提升（除非阻塞 §4 验收）
- PR 上游 / staging 部署
- plugin SDK 接口扩展（除非 OpenFrontendFile 暴露不足）
- host i18n 文案的多语种新增

## 6. 给 Designer 的提问

1. **bundle 形态**：plugin entry.js 走 vite library 单文件（`format: 'es'`, no code-splitting），还是允许 split chunks？后者会让 OpenFrontendFile 需要返回任意子文件。建议先选单文件，与 hello-world 对齐。
2. **Vue / pinia 外部化**：当前 vite.config 已 `external: ['vue', 'vue-router', 'vue-i18n', 'pinia', 'axios']`，但 hello-world entry 是手写的、完全不 import vue。channel-management 用了 SFC，编译后会有 `import { defineComponent } from 'vue'`。runtime 必须保证宿主把 vue 暴露成 ESM 可解析的 specifier（importmap？还是 sdk.vue？）。请明确加载契约。
3. **Dockerfile 构建图**：
   a) plugin-frontend 构建是放在现有 `plugin-builder` stage 加 node 工具链，还是单独新增 `plugin-frontend-builder` stage？
   b) 与 host frontend-builder 是否共享 pnpm store / lockfile？
   c) plugin 自带 `package.json` + workspace 是否要加进 root `pnpm-workspace.yaml`？
4. **i18n 命名空间策略**：host 当前 `admin.channels.*` keys 是否还有非渠道页面引用？plugin 选择 namespace `channel-management`（已在 manifest）后，host 端的 `admin.channels.*` keys 是删除、保留兜底，还是迁移为 `channel-management.*` 让 plugin 注册时合并？
5. **路由占位**：删除 host 静态路由后，host loader 在 plugin 未启动 / 加载失败时如何兜底 `/admin/channels` 访问？是否依靠 `loader.ts` 占位 + PluginView 降级？需明确"加载失败时显示什么 UI"。
