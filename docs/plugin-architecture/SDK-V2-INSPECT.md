# SDK-V2 Inspector 审计报告

> 时间 2026-04-26  •  基线 `feat/plugin-system-fixes` (`d0c20fed`)  •  分支
> `feat/plugin-system-fixes--sdk-v2-inspect`
>
> 任务: 在不修改任何代码的前提下, 调查 V1 残留的 host 源码耦合, 给
> Designer / Planner 提供"哪些组件入 SDK / 怎么入 / 体量多大"的判断依据.

---

## 0. V1 现状与"妥协"复述

V1 (commit `fedc8638` body) 自承的妥协: PLAN T2 验收要求 plugin 内 `from "@/"`
的命中数 = 0, 实际保留了 16 处 host import. 取而代之用 vite alias + 自写
rollup plugin (`hostNodeModulesResolver`, `publicAssetStub`, 见
`plugins/channel-management/frontend/vite.config.ts:107-165`) 把 host 源码
inline 进 plugin bundle, 代价是 dist/entry.js = 4.8 MB (gzip 1 MB).

V2 任务前提即"消除上述 inline", 改用 SDK 包暴露通用组件.

---

## 1. plugin 实际 host 依赖清单

证据全部来自 `Grep "from \"@/" plugins/channel-management/frontend/src` (按
路径分组). plugin 内 `from "../../../frontend/"` = 0 行 (Grep 命中 0), 唯一
跨 worktree 的相对 import 是 `src/index.ts:20` 的
`"../../../../frontend/src/plugins/sdk/host-sdk"` (类型 only, 不进 bundle).

| host 模块路径                                  | 引用类型      | plugin 引用 | host 其他地方引用 | 分类  |
| ---------------------------------------------- | ------------- | ----------- | ----------------- | ----- |
| `@/components/common/DataTable.vue`            | 通用表格      | 1           | 16 (16 文件)      | A |
| `@/components/common/Pagination.vue`           | 通用分页      | 1           | 36 (26 文件)      | A |
| `@/components/common/BaseDialog.vue`           | 通用 Modal    | 1           | 88 (88 文件)      | A |
| `@/components/common/ConfirmDialog.vue`        | 确认 Modal    | 1           | (subset of 235, 见 1.1) | A |
| `@/components/common/EmptyState.vue`           | 空态占位      | 1           | (subset of 235)   | A |
| `@/components/common/Select.vue`               | 通用下拉      | 2 (Channels + PricingEntryCard) | 41 (40 文件) | A |
| `@/components/common/PlatformIcon.vue`         | 平台图标      | 1           | (subset of 235)   | A |
| `@/components/common/Toggle.vue`               | 通用开关      | 1           | (subset of 235)   | A |
| `@/components/common/types` (`Column`)         | 类型定义      | 1           | DataTable 内部    | D |
| `@/components/icons/Icon.vue`                  | 图标渲染器    | 4 (Channels/Pricing/ModelTag/Interval) | 235 (97 文件) | A |
| `@/components/layout/AppLayout.vue`            | host 整页 chrome | 1        | 26 (26 文件)      | B |
| `@/components/layout/TablePageLayout.vue`      | 表格页 slot 容器 | 1        | 26 (26 文件)      | A |
| `@/composables/usePersistedPageSize`           | localStorage 包装 | 1       | (subset of 235)   | D |
| `@/stores/app`  (`useAppStore`)                | host pinia store | 1       | host 全局         | C |
| `@/api/admin`   (`adminAPI.groups`)            | host API client | 1         | host 全局         | C |
| `@/types`       (`AdminGroup`, `GroupPlatform`) | host 类型     | 2 (ChannelsView + host-modules.d.ts) | host 全局 | D |

证据原始位置 (举三例, 完整列表共 18 行 import):
- `plugins/channel-management/frontend/src/views/ChannelsView.vue:425-445`
  (15 处 host import)
- `plugins/channel-management/frontend/src/components/PricingEntryCard.vue:232-233`
  (Select + Icon)
- `plugins/channel-management/frontend/src/host-modules.d.ts:14-83`
  (类型 shim, 8 个 declare module 描述了 plugin 信任的最小契约)

`@/plugins/sdk/*` 命中 = 0 行 (`Grep "from \"@/plugins/sdk" plugins/channel-management/frontend/src` => "No matches found"). plugin 完全没用现有 SDK, 这是 V1 留的 bug 形入口, 见 §5.

### 1.1 host 通用组件被引用密度量级

`Grep` 在 `frontend/src` 上的 `Pagination|BaseDialog|ConfirmDialog|EmptyState|...`
聚合: 235 occurrences across 97 files. 每一项命中均覆盖 host 多处视图,
全是合格的"通用层"组件, 适合作为 SDK 公共面.

---

## 2. A 类组件的"如果进 SDK, 会带出哪些副作用"

(每条结尾"传递依赖"是从该 host 文件继续 `Grep "from \"@/\"" <file>` 得到的.)

| 候选 SDK 组件      | 自身 host import 链 (一层) | 实际副作用 |
| ------------------ | -------------------------- | ---------- |
| DataTable.vue      | `@/components/icons/Icon.vue` (DataTable.vue:202) | 强制带 Icon |
| Select.vue         | `@/components/icons/Icon.vue` (Select.vue:111)    | 强制带 Icon |
| BaseDialog.vue     | `@/components/icons/Icon.vue` (BaseDialog.vue:46) | 强制带 Icon |
| Pagination.vue     | `@/components/icons/Icon.vue` (Pagination.vue:123) + `@/utils/tablePreferences` (line 125) | 带 Icon + tablePreferences |
| ConfirmDialog.vue  | (Grep 命中 0 行 `@/`)      | 无 host 内部 dep, 但其底层 `<BaseDialog>` 编译产物会嵌入 |
| EmptyState.vue     | `@/components/icons/Icon.vue` (EmptyState.vue:58) | 强制带 Icon |
| PlatformIcon.vue   | `@/types` 中的 `GroupPlatform` (line 32) | 仅类型, 零运行时副作用 |
| Toggle.vue         | (Grep 命中 0 行)           | 自包含 |
| Icon.vue           | (Grep 命中 0 行 `@/`; 内部走 `lucide-vue-next`) | 自包含, 是其它 A 类的根 |
| TablePageLayout.vue| (Grep 命中 0 行 `@/`)      | 纯 slot 容器, 自包含 |

结论: A 类的真实根依赖只有两个 — `Icon.vue` (传递性最高) 与
`utils/tablePreferences.ts`. 把这 10 个 A 类组件 + `Icon.vue` + tablePreferences
搬进 SDK 包, 即可让 plugin 完全脱离 host vite alias.

不入 SDK 的反例: `AppLayout.vue` (B 类) 自身依赖
`@/components/layout/AppSidebar.vue` (整套 host 导航 chrome) +
`@/stores/auth` + `@/composables/useOnboardingTour` (`AppLayout.vue:7,28-31`).
plugin 不应把 host 整页框架塞进自己的 bundle —— 实际 plugin 是被 host
`PluginView.vue` 包在 AppLayout 里渲染的, 此处属误用 (V1 历史遗留, 见
`docs/plugin-architecture/INSPECT.md:65` "plugin 内已存在等价副本"段).

---

## 3. 推荐 SDK 包形态

### 3.1 位置 (待 Designer 决议, Inspector 倾向最后一项)

| 选项                                 | 优点 | 缺点 |
| ------------------------------------ | ---- | ---- |
| `plugin-sdk-frontend/` (顶级)        | 与 Go SDK (`plugin-sdk/`) 视觉对等; 显式独立 npm 包 | 跨 frontend 与 plugin frontend 必须走 npm publish 或 file: 协议 |
| `plugin-sdk/frontend/` (合在 Go SDK) | 一处目录 = 一个 SDK 概念 | Go module 与 npm package 混在一个目录, 看 `plugin-sdk/go.mod` 的人会困惑 |
| **`frontend/packages/plugin-sdk/`**  | 启用 pnpm workspace, host frontend 与 plugin 都 `import "@sub2api/plugin-sdk"` 走本地 link, 无 publish | 需要新建 `frontend/pnpm-workspace.yaml` 或把现有 host frontend 改造成 monorepo 子包 |

证据: `frontend/package.json` 是 host frontend 的 root, 顶层
没有 `pnpm-workspace.yaml` (Glob 结果 = 0 命中, 项目内 maxdepth 3 内
仅 `frontend/package.json` 一处 package.json). 所以走 monorepo 改造成本是
"新建 1 个 yaml + 改 host package.json 加 workspaces 字段".

### 3.2 给 Designer 的建议

1. 包内导出: 不要导出 Vue 实例本身, 只导出 SFC 组件
   (`DataTable, Select, ...`) + 类型 + utils. 运行时仍走
   `window.__SUB2API_HOST_VUE__` 由现有 importmap 解析.
2. build target: SDK 包以源码形式 (`*.vue` + `*.ts`) 发布, 不预编译.
   plugin vite 自己 transform, host 也自己 transform. 这样组件保留
   `<style scoped>` + tailwind class 不双份编译.
3. Icon 是种子组件: 把 `Icon.vue` 与 `lucide-vue-next` 一起放进 SDK
   peerDependencies, 确保 host + plugin 共用同一份 lucide.

---

## 4. importmap 扩展接口契约草案

### 4.1 已建立的基础设施 (V1, 直接复用)

- `frontend/src/plugins/sdk/expose-runtime.ts:32-43` 通过
  `__SUB2API_HOST_*__` 暴露 vue/vue-router/vue-i18n/pinia/axios 五个单例.
- `backend/internal/server/routes/plugin_assets.go:47-185` 有 `sharedRuntimeReExport`
  map, 处理 `/api/v1/plugin-assets/__shared__/<name>.js`. 路由保留
  `__shared__` 字段名 (`plugin_assets.go:229`), 不依赖 PluginManager
  (即使插件系统初始化失败也可工作).

### 4.2 V2 扩展点 (复用 V1 模式, 不需要新增 endpoint)

| 维度                | 复用 / 新增 | 证据 / 备注 |
| ------------------- | ----------- | ----------- |
| 后端 endpoint       | 复用 `/api/v1/plugin-assets/__shared__/*` | 仅在 `sharedRuntimeReExport` map 添加一条 `"plugin-sdk.js"`: ESM 源码. `plugin_assets.go:189-205` 的 `servePluginSharedAsset` 已经按 key 查表返回, 零路由变动. |
| importmap 注入      | 复用现有注入流 (host index.html) | 增加一条 `"@sub2api/plugin-sdk"`: `/api/v1/plugin-assets/__shared__/plugin-sdk.js`. 注入位置待 Designer 决议 (V1 design §3 第 127-141 行预留). |
| 浏览器侧解析        | 原生 importmap | `"@sub2api/plugin-sdk"` -> `__shared__/plugin-sdk.js` -> 该 ESM 内部 `export const DataTable = window.__SUB2API_HOST_SDK__.components.DataTable` |
| `host-sdk-window.ts`| 需要扩展: 把 components 也挂上 window | `host-sdk.ts:151-163` 的 `HostSdk` 接口当前只有 theme/font/i18n/notify/router/auth/http/vue, 需要新增 `components: HostComponents`. `attachHostSdkToWindow` (`host-sdk-window.ts:20-30`) 把整个 sdk 写到 `window[HOST_SDK_GLOBAL_KEY]`, 自动覆盖. |
| 前端 host 入口      | 在 `createHostSdk` (`host-sdk-impl.ts`) 中静态 `import { DataTable } from "@/components/common/DataTable.vue"` 并塞进 `sdk.components` | 这是唯一让 host 把组件单例 bind 到 window 的位置. |

### 4.3 接口契约 (待 Designer 决议)

```ts
// 待 Designer 决议: 字段名是否走 sdk.components.<name> 还是 sdk.ui.<name>
interface HostComponents {
  DataTable: Component
  Pagination: Component
  Select: Component
  BaseDialog: Component
  ConfirmDialog: Component
  EmptyState: Component
  Toggle: Component
  PlatformIcon: Component
  TablePageLayout: Component
  Icon: Component  // 必须暴露, 见 §2
  // 待 Designer 决议是否暴露 utils
  utils?: { tablePreferences: typeof import('@/utils/tablePreferences') }
}
```

---

## 5. 给 Planner 的输入

### 5.1 数量摘要

- A 类总数: 10 (Icon + 9 个公共组件含 TablePageLayout, 见 §1)
  -> SDK 包初始体量 = 10 个 SFC + 1 个 utils + 1 个类型文件 ~= 1.5-2 MB 源码
  (与 V1 inline 4.8 MB bundle 相比可削减约 60%).
- B 类: 1 (`AppLayout.vue`) — plugin 应删除这条 import,
  plugin entry 不该自己装载 host 整页 chrome (它本来就在 PluginView 里).
- C 类: 2 (`@/stores/app`, `@/api/admin.groups`) — plugin 应改为
  `sdk.notify.error(...)` (替换 useAppStore.showError) 与
  `sdk.http.apiClient.get("/admin/groups")` (替换 adminAPI.groups).
  现有 SDK (`host-sdk.ts:151-163`) 已经有 `notify` 与 `http`, 改 plugin
  代码即可, 不需要 host 侧动.
- D 类: 3 (`Column` 类型, `usePersistedPageSize`, `AdminGroup/GroupPlatform`
  类型) — 建议复制到 plugin 或同步进 SDK 包 d.ts 文件,
  避免运行时依赖.

### 5.2 风险点

- Icon.vue 是关键种子: 几乎所有 A 类组件都依赖它 (§2). 如果 Designer
  决定 SDK 不暴露 Icon, A 类全部跟着崩 — 强制必须暴露.
- Pagination 反向依赖 utils/tablePreferences: 它进了 SDK, tablePreferences
  也得跟进, 否则 plugin build 仍然 `from "@/utils/tablePreferences"`.
- i18n 子键耦合: ChannelsView.vue 多处 `t("admin.channels.*")`,
  当前由 plugin 自己 `i18n/{en,zh}.ts` 注册到 `admin.channels.*` 子树
  (`src/index.ts:41-44`). 如果 SDK 内的组件 (BaseDialog/ConfirmDialog) 内部
  引用了 `t("common.confirm")` 等 host i18n key, plugin 必须保证 host
  已经提供这些键 — 当前 host 提供 (V1 已运行), 但 SDK 把组件抽出后
  Designer 需要把 SDK 组件依赖的 host i18n key 集合显式列出来.
- SDK 包发布形态尚未决议: pnpm workspace 改造涉及 host frontend
  build 链 (`frontend/package.json`, dockerfile frontend-builder stage), 不只是
  新增一个目录. 见 §3.1, 需要 Designer + Planner 协同.
- A 类组件之间没有 cyclic import (上表 §2 检查过), 进 SDK 不会触发
  循环依赖, 这是低风险项.

---

## 6. 结论一句话

V1 plugin 实际只用了 host 的 10 个通用 UI 组件 (A 类) + 1 个工具函数 +
3 类类型, 真正的"业务耦合点"只有 2 处 (notify store + admin.groups API),
这两处现有 SDK 的 `notify` / `http.apiClient` 已经能替代; SDK 包初始
体量 ~= 10 个 SFC + Icon, 走 importmap `__shared__/plugin-sdk.js` 复用现有
后端 endpoint, 与 V1 的 `__SUB2API_HOST_*__` 暴露方式同型, 无需新增
后端路由, 仅扩展 `host-sdk.ts` 接口加一个 `components` 字段.
