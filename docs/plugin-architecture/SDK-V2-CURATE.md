# SDK-V2 Curator 决策书

> 时间 2026-04-26  •  基线 `feat/plugin-system-fixes` (`dde391ac`, 已合并 V2 Inspector)  •  分支
> `feat/plugin-system-fixes--sdk-v2-curate`
>
> 角色: Planner + Designer 合并. 上游输入 = `SDK-V2-INSPECT.md`. 下游产出 =
> Implementer 可直接抄的执行单. **拒绝完美主义**: 抽 5-8 个真正稳定且双向广用的
> 通用组件; 业务组件该复制就复制, 纯函数该 inline 就 inline.

---

## 1. 三问审计 (核心决策)

对 Inspector 列出的全部候选, 按"半年 API 稳定性 / host ≥3 处 + plugin ≥1 处复用 /
现有 SDK reactive 能力是否可替代"三问打标签.

| 候选 | 稳定 | host 复用 | plugin 复用 | SDK 替代 | 决策 | 理由 |
| --- | --- | --- | --- | --- | --- | --- |
| Icon.vue | 稳 | 235 处 / 97 文件 | 4 处 | 否 (lucide 是 peerDep, 自造无意义) | **抽 (Tier-1)** | 所有 A 类组件的种子, 不抽则其它全崩 (Inspector §2) |
| DataTable.vue | 稳 | 16 文件 | 1 处 | 否 | **抽 (Tier-1)** | 表格是数据展示标配, 内部接 Icon, 复制成本高 (~600 行) |
| BaseDialog.vue | 稳 | 88 文件 | 1 处 | 否 (sdk.notify 只覆盖 toast 不覆盖 modal) | **抽 (Tier-1)** | Modal 是交互标配, 复用面最广 |
| Select.vue | 稳 | 41 处 / 40 文件 | 2 处 (Channels + PricingEntryCard) | 否 (原生 select 风格不一致) | **抽 (Tier-1)** | 下拉是表单标配, host 自定义样式必须保持一致 |
| Pagination.vue | 稳 | 36 处 / 26 文件 | 1 处 | 否 | **抽 (Tier-1)** | 与 DataTable 配套出现, 抽一个不抽另一个会逼 plugin 自造 |
| ConfirmDialog.vue | 稳 | (subset 235) | 1 处 | 否 (没有 sdk.confirm) | **抽 (Tier-2)** | 体量小 (~60 行), 但 plugin 删除流必用; 不抽 plugin 会复制 |
| EmptyState.vue | 稳 | (subset 235) | 1 处 | 否 | **抽 (Tier-2)** | 极小 (~30 行), 但与 DataTable 强耦合 (空表渲染) |
| Toggle.vue | 稳 | (subset 235) | 1 处 | 否 (原生 checkbox 风格不一致) | **抽 (Tier-2)** | 自包含无副作用, 抽入成本几乎为零 |
| PlatformIcon.vue | 稳 | (subset 235) | 1 处 | 否 (业务图标映射, 内部依赖 lucide) | **抽 (Tier-2)** | host 维护平台→图标映射, plugin 复制会失同步 |
| TablePageLayout.vue | 稳 | 26 文件 | 1 处 | 部分 (8 行 div + slot 自己写也行) | **不抽 (砍)** | Inspector 列 A, 但 plugin 只用 1 处, 自己写 `<div class="...">` 即可省 1 个文件 |
| AppLayout.vue | 不稳 | 26 文件 | 1 处 (误用) | n/a | **不抽 (删 import)** | B 类: plugin 已经被 PluginView 包在 AppLayout 里, 这是 V1 历史遗留误用 |
| `@/stores/app` (useAppStore) | 稳 | host 全局 | 1 处 | **是**: `sdk.notify.error/success/warning/info` 已存在 | **不抽 (改 plugin)** | C 类: 改 plugin 调用为 `sdk.notify.*` |
| `@/api/admin` (adminAPI.groups) | 业务路径稳 | host 全局 | 1 处 | **是**: `sdk.http.apiClient.get('/admin/groups')` | **不抽 (改 plugin)** | C 类: plugin 改成直接用 SDK axios |
| `@/types` (AdminGroup, GroupPlatform) | 稳 | host 全局 | 2 处 | n/a (类型不进 bundle) | **不抽 (复制类型)** | D 类: plugin 复制 5 行 interface 即可, 无运行时副作用 |
| `@/components/common/types` (Column) | 稳 | DataTable 内部 | 1 处 | n/a | **抽 (随 DataTable 走)** | DataTable 进 SDK 时把 `Column` 类型一并 re-export |
| `@/composables/usePersistedPageSize` | 稳 | (subset 235) | 1 处 | 否 (但 1 行 localStorage 调用) | **不抽 (inline)** | D 类工具: 整个函数 ~10 行, plugin 自己写 1 行即可 |
| `@/utils/tablePreferences` | 稳 | Pagination 内部 | 0 处 (传递依赖) | 否 | **抽 (随 Pagination 走)** | Pagination 进 SDK 必须带它, 不带则编译失败 |

### 1.1 Tier-1 (5 个) — 必抽

`Icon.vue` + `DataTable.vue` + `BaseDialog.vue` + `Select.vue` + `Pagination.vue`

- 通过率: 半年内 API 都不会大改 (host 自身已用 88-235 次, 改一次 = 改全站)
- 双向: host 全部 ≥16 处, plugin ≥1 处
- 不可替代: 现有 SDK reactive (auth/theme/notify/router/http/i18n/vue) 不覆盖 UI 组件
- 砍掉一个的代价: plugin 自造 ~60-600 行, 且与 host 视觉脱节

### 1.2 Tier-2 (4 个) — 跟着 Tier-1 一起抽

`ConfirmDialog.vue` + `EmptyState.vue` + `Toggle.vue` + `PlatformIcon.vue`

- 不是 must-have, 但**和 Tier-1 强配套** (DataTable→EmptyState, BaseDialog→ConfirmDialog,
  PlatformIcon 是平台数据展示标配)
- 体量都小 (30-100 行), 抽入成本几乎为零
- 砍掉它们的代价: plugin 复制 ~200 行, 风格与 host 同步要靠人盯

### 1.3 砍掉 (3 个候选, 节省 ~700 行 SDK 代码)

| 候选 | 砍后 plugin 多写 |
| --- | --- |
| TablePageLayout.vue | ~10 行 (一个 `<div class="space-y-4 p-6"><slot/></div>` 包装) |
| AppLayout.vue (B 类) | 0 行 (删 import 即可, plugin 本来就在 PluginView 内) |
| usePersistedPageSize | ~3 行 (localStorage.getItem 包装) |

### 1.4 工具/类型处理

- `Column` 类型: 随 DataTable 一起 re-export
- `tablePreferences.ts`: 随 Pagination 一起搬入 SDK (零业务依赖, 5 个函数)
- `AdminGroup / GroupPlatform`: plugin 自己写 5 行 interface (业务类型, 不该入 SDK)

### 1.5 最终决策

**抽 9 个 SFC + 1 个 utils + 1 个类型, 共 11 个文件.**

(Tier-1 的 5 个 + Tier-2 的 4 个 = 9 SFC; tablePreferences.ts 1 utils; Column 类型 1 文件)

砍 Inspector 列出的 1 个 A (TablePageLayout) + 1 个 B (AppLayout) + 1 个 D
(usePersistedPageSize) = 3 个候选.

---

## 2. SDK 包工程化决策

### 2.1 包路径 / 包名

- **路径**: `frontend/packages/plugin-sdk/`
- **包名**: `@sub2api/plugin-sdk`
- **理由**: Inspector §3.1 三选一中倾向 monorepo 方案. 实测 root 无 `pnpm-workspace.yaml`,
  `frontend/package.json` 是唯一 npm 包. 走 pnpm workspace 改造成本最低 (新增一个
  yaml + 改 host package.json), 比顶级 `plugin-sdk-frontend/` 更不易与 Go SDK
  混淆, 比 `plugin-sdk/frontend/` 更不会让看 `plugin-sdk/go.mod` 的人困惑.

### 2.2 workspace 配置

新建 `frontend/pnpm-workspace.yaml`:

```yaml
packages:
  - '.'              # host frontend (仍是 root, 保留所有现有 scripts)
  - 'packages/*'     # @sub2api/plugin-sdk 与未来潜在子包
```

`frontend/package.json` dependencies 加一行:

```json
"@sub2api/plugin-sdk": "workspace:*"
```

让 host 自己也通过 SDK 包路径引用这些组件 (单一来源真相, plugin 与 host 同步演化).

### 2.3 SDK 包形态

`frontend/packages/plugin-sdk/package.json`:

```json
{
  "name": "@sub2api/plugin-sdk",
  "version": "1.0.0",
  "type": "module",
  "main": "./src/index.ts",
  "types": "./src/index.ts",
  "exports": {
    ".": "./src/index.ts",
    "./components/*": "./src/components/*",
    "./types": "./src/types/index.ts"
  },
  "peerDependencies": {
    "vue": "^3.4.0",
    "lucide-vue-next": "^0.300.0"
  }
}
```

**源码形态发布** (Inspector §3.2 决议): `*.vue` + `*.ts` 不预编译, 让 host vite
与 plugin vite 各自 transform. tailwind class 与 `<style scoped>` 走各自 PostCSS
管线, 不双份编译.

### 2.4 运行时导出方式 — 重要决策

Inspector §4.3 草案给了两条路: (a) 通过 importmap + `window.__SUB2API_HOST_SDK__.components.*`
注入, (b) plugin 直接 `import` SDK 包源码.

**决策: 走 (b), 直接 import + 扩展 importmap, 不挂 `window.__SUB2API_HOST_SDK__.components`.**

理由:
- (a) 需要 host 启动时把每个组件 bind 到 window, plugin 通过 ESM proxy 取回, 多一跳
  序列化死代码, 且组件无法 tree-shake
- (b) plugin `import { DataTable } from '@sub2api/plugin-sdk'` 直接走 importmap,
  importmap 把 `@sub2api/plugin-sdk` 解析到 `/api/v1/plugin-assets/__shared__/plugin-sdk.js`,
  这个 ESM 内部 re-export 即可
- 与 V1 vue/pinia/axios 暴露模式同型 (Inspector §4.2), 不破坏已建立的契约

**实现路径**: host frontend 多一个 build entry, 输出 `frontend/dist/plugin-sdk.js`
(单文件 ES bundle, vue + lucide 仍 external). 后端 `servePluginSharedAsset` 加一个
特例分支, 当 `asset == "plugin-sdk.js"` 时, 走 embed FS 读 `dist/plugin-sdk.js`
返回 (而不是从手写的 `sharedRuntimeReExport` map 里取代码字符串 — SDK 包是 vite
编译产物, 不适合塞进 Go map).

### 2.5 `host-sdk.ts` 是否扩展 `components` 字段

**决策: 不加 `components` 字段到 `HostSdk` interface.**

理由: SDK 组件通过 importmap 直接 import, 不需要再绕一层 `sdk.components.DataTable`.
保持 `HostSdk` 接口聚焦 reactive 能力 (auth/theme/notify/router/http/i18n/vue).
组件入口 = `import { DataTable } from '@sub2api/plugin-sdk'`, 与 vue 同型.

### 2.6 importmap 扩展

`backend/internal/web/embed_on.go` 第 223-230 行的 `pluginImportMap` 加一条:

```go
const pluginImportMap = `<script type="importmap">{"imports":{` +
    `"vue":"/api/v1/plugin-assets/__shared__/vue.js",` +
    `"vue-router":"/api/v1/plugin-assets/__shared__/vue-router.js",` +
    `"vue-i18n":"/api/v1/plugin-assets/__shared__/vue-i18n.js",` +
    `"pinia":"/api/v1/plugin-assets/__shared__/pinia.js",` +
    `"axios":"/api/v1/plugin-assets/__shared__/axios.js",` +
    `"@sub2api/plugin-sdk":"/api/v1/plugin-assets/__shared__/plugin-sdk.js"` + // 新增
    `}}</script>`
```

后端 `plugin_assets.go` 的 `servePluginSharedAsset` 加一个特例分支: 当
`asset == "plugin-sdk.js"` 时, 直接走 host frontend embed FS 返回 `dist/plugin-sdk.js`.

---

## 3. Implementer 执行清单

### Step 1 — 建 SDK 包 (10 分钟)

```
frontend/
├── pnpm-workspace.yaml          # 新建
└── packages/
    └── plugin-sdk/              # 新建
        ├── package.json         # 见 §2.3
        ├── README.md            # 一段说明 + 公开组件清单
        └── src/
            ├── index.ts         # named re-export 入口
            ├── components/      # 9 个 SFC
            ├── utils/
            │   └── tablePreferences.ts
            └── types/
                └── index.ts     # Column + 公开类型
```

`packages/plugin-sdk/src/index.ts`:

```ts
export { default as Icon } from './components/Icon.vue'
export { default as DataTable } from './components/DataTable.vue'
export { default as BaseDialog } from './components/BaseDialog.vue'
export { default as ConfirmDialog } from './components/ConfirmDialog.vue'
export { default as Select } from './components/Select.vue'
export { default as Pagination } from './components/Pagination.vue'
export { default as EmptyState } from './components/EmptyState.vue'
export { default as Toggle } from './components/Toggle.vue'
export { default as PlatformIcon } from './components/PlatformIcon.vue'
export type { Column } from './types'
```

### Step 2 — 复制 11 个文件 (30 分钟)

src → dst 清单:

| 源 | 目标 |
| --- | --- |
| `frontend/src/components/icons/Icon.vue` | `frontend/packages/plugin-sdk/src/components/Icon.vue` |
| `frontend/src/components/common/DataTable.vue` | `.../components/DataTable.vue` |
| `frontend/src/components/common/BaseDialog.vue` | `.../components/BaseDialog.vue` |
| `frontend/src/components/common/ConfirmDialog.vue` | `.../components/ConfirmDialog.vue` |
| `frontend/src/components/common/Select.vue` | `.../components/Select.vue` |
| `frontend/src/components/common/Pagination.vue` | `.../components/Pagination.vue` |
| `frontend/src/components/common/EmptyState.vue` | `.../components/EmptyState.vue` |
| `frontend/src/components/common/Toggle.vue` | `.../components/Toggle.vue` |
| `frontend/src/components/common/PlatformIcon.vue` | `.../components/PlatformIcon.vue` |
| `frontend/src/utils/tablePreferences.ts` | `.../utils/tablePreferences.ts` |
| `frontend/src/components/common/types.ts` | `.../types/index.ts` |

复制后, **改源文件内 `@/` import** 为 SDK 包内相对路径. 例如 `Pagination.vue` 内
`from '@/components/icons/Icon.vue'` → `from './Icon.vue'`,
`from '@/utils/tablePreferences'` → `from '../utils/tablePreferences'`.
`PlatformIcon.vue` 内 `from '@/types'` 改为本地内联 `type GroupPlatform = string`
(避免反向耦合 host types).

**host 保留 vs 删除**: 复制完成后, 把 host `frontend/src/components/common/*.vue`
改为 `export * from '@sub2api/plugin-sdk'` 单行 re-export, 让 host 现有 235 处
`from '@/components/common/...'` 零修改继续工作. 避免本次 PR 范围扩散到全 host
改动.

### Step 3 — host frontend 加 SDK build entry (15 分钟)

新建 `frontend/vite.sdk.config.ts`:

```ts
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

export default defineConfig({
  plugins: [vue()],
  build: {
    outDir: 'dist',
    emptyOutDir: false,           // 不清掉 host bundle
    lib: {
      entry: resolve(__dirname, 'packages/plugin-sdk/src/index.ts'),
      formats: ['es'],
      fileName: () => 'plugin-sdk.js',
    },
    rollupOptions: {
      external: ['vue', 'lucide-vue-next'],
    },
  },
})
```

`frontend/package.json` scripts 加:

```json
"build:sdk": "vite build -c vite.sdk.config.ts",
"build": "vue-tsc -b && vite build && pnpm build:sdk"
```

### Step 4 — 后端 serve `plugin-sdk.js` (15 分钟)

`backend/internal/server/routes/plugin_assets.go` 的 `servePluginSharedAsset` 改:

```go
func servePluginSharedAsset(c *gin.Context, asset string) {
    if asset == "plugin-sdk.js" {
        servePluginSdkBundle(c)   // 新增: 走 embed FS 读 dist/plugin-sdk.js
        return
    }
    body, ok := sharedRuntimeReExport[asset]
    // ... 原逻辑
}
```

`backend/internal/web/embed_on.go` 的 `pluginImportMap` 加 `@sub2api/plugin-sdk`
条目 (见 §2.6).

### Step 5 — 改 plugin imports (40 分钟)

`plugins/channel-management/frontend/src/views/ChannelsView.vue` 把 9 个 host
组件 import 改成 SDK 单行:

```ts
// 删
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
// ... 共 9 行
import type { Column } from '@/components/common/types'

// 改为
import {
  DataTable, Pagination, BaseDialog, ConfirmDialog, EmptyState,
  Select, Icon, Toggle, PlatformIcon,
  type Column,
} from '@sub2api/plugin-sdk'
```

同样 `PricingEntryCard.vue` 的 Select + Icon 改成 SDK import.

**剩余 4 类 import 处理**:
- `import AppLayout from '@/components/layout/AppLayout.vue'` → **删除**, plugin
  在 PluginView 内, 不需要再套一层 (Inspector §2 已说明)
- `import TablePageLayout from '@/components/layout/TablePageLayout.vue'` → **改为
  inline `<div class="space-y-4 p-6"><slot/></div>`**, 砍 1 个 host 依赖
- `useAppStore` → 改 `sdk.notify.error/success/warning/info` (V1 SDK 已经覆盖)
- `adminAPI.groups.getAll()` → 改 `sdk.http.apiClient.get('/admin/groups').then(r => r.data)`
- `getPersistedPageSize()` → inline 1 行
  `Number(localStorage.getItem('table.pageSize')) || 20`
- `AdminGroup / GroupPlatform` 类型 → plugin 本地
  `interface AdminGroup { id: number; name: string; platform: string }` (5 行)

`plugins/channel-management/frontend/package.json` 加一行:
```json
"@sub2api/plugin-sdk": "workspace:*"
```

### Step 6 — 清除 V1 vite 折中 (15 分钟)

`plugins/channel-management/frontend/vite.config.ts` 删:
- alias `'@'` → host frontend src (整段 `resolve.alias` 删除)
- `hostNodeModulesResolver` 整段函数 (97-129 行) 删除
- `publicAssetStub` 整段函数 (95-129 行) 删除 (plugin 不再 inline host 图片)
- `rollupOptions.plugins: [hostNodeModulesResolver(...), publicAssetStub()]` 整行删除
- `rollupOptions.external` 加一条 `'@sub2api/plugin-sdk'`

`plugins/channel-management/frontend/src/host-modules.d.ts` 整个文件删掉
(SDK 包带的真实类型替代 stub shim).

### Step 7 — 验收

执行 §4 验收清单.

---

## 4. 验收清单

- [ ] `pnpm install` 在 root frontend 成功 (workspace 解析 @sub2api/plugin-sdk)
- [ ] `cd frontend && pnpm build` 成功, 输出含 `dist/plugin-sdk.js`
- [ ] `cd plugins/channel-management/frontend && pnpm build` 成功, dist/entry.js
      体积 ≤ 1.5 MB (V1 baseline 4.8 MB; 砍掉 inline host 组件 + transitive deps)
- [ ] `grep -rn "from '@/" plugins/channel-management/frontend/src/` 命中 = 0 行
      (除允许的少数: 0 行)
- [ ] `grep -rn "from '../../../frontend" plugins/channel-management/frontend/src/`
      命中 = 0 行 (V1 唯一 type-only import 也删掉, 改用 `@sub2api/plugin-sdk`)
- [ ] `plugins/channel-management/frontend/src/host-modules.d.ts` 已删除
- [ ] `plugins/channel-management/frontend/vite.config.ts` 不含
      `hostNodeModulesResolver` 与 `publicAssetStub`
- [ ] 浏览器手动验证: 打开 `/admin/channels`, DataTable 渲染、Pagination 翻页、
      Select 下拉、ConfirmDialog 删除二次确认、空状态 EmptyState 全部正常
- [ ] DevTools Network: `/api/v1/plugin-assets/__shared__/plugin-sdk.js` 200 响应,
      Content-Type `application/javascript`, ETag 命中后 304
- [ ] `host-sdk.ts` interface 未变 (没新增 `components` 字段, 保持向后兼容)
- [ ] `frontend/dist/plugin-sdk.js` 走 host vite tree-shake, 体积 ≤ 200 KB
      (9 个 SFC + 5 个 util 函数)

---

## 5. 给 Implementer 的 Sanity Check (开工前必验)

1. **workspace 路径决议**: 在 `frontend/` 跑 `pnpm install`, 确认现有 host
   build 不被改动. 如果 root (`/`) 还有别的 yarn/npm lock 与 frontend 冲突,
   先在 PR 描述里说明, 不要硬上 monorepo.
2. **SDK build 产物路径**: 确认 `frontend/dist/plugin-sdk.js` 在 Dockerfile 的
   `frontend-builder` stage 能被 embed 进后端二进制. 如果当前 embed 只 walk
   `frontend/dist/index.html` + 静态资产, 需要扩 embed FS 路径让
   `plugin-sdk.js` 也被打包 (检查 `backend/internal/web/embed_on.go` 的
   `embed.FS` 声明).

如以上两点之一不成立, 先停下来在 PR 描述里说明, 不要硬改 build 链.
