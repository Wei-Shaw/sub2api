# V3 Curator 决策书 — 机械清理 (Mechanical Cleanup)

> 时间 2026-04-26  •  基线 `feat/plugin-system-fixes` (`fd1a62e4`, V3 Inspector 已合并)
> 子分支 `feat/plugin-system-fixes--v3-curate`
>
> 角色: Planner + Designer 合并. 上游输入 = `V3-INSPECT.md`. 下游产出 = Implementer
> 高度可抄的执行单. **本期边界**: 消除 V2 留下的两项妥协 (host common 字节级副本 +
> plugin 跨目录 type-only 引用). 不做 host-only 组件 SDK 化, 不做业务类型 SDK 化.

---

## 1. 六议题决策

| # | 议题 | 决策 | 理由 |
| - | --- | --- | --- |
| 1 | host common .vue 副本删除 | **(a) 删除 host 副本 + 改 168 处 import** | Inspector §1 已证明 6 处 DIFF 全是路径改写 + 1 处类型收缩, 0 业务漂移; §2 证明 168/168 可 sed; 0 dynamic import / 0 测试 / 0 defineAsyncComponent |
| 2 | host common .vue 改为代理组件 | **(a) 直接删** | Inspector §2 证明 168 处 import 都是规则的 `default import` 形式, sed 100% 可改; 留代理 = 净增 9 个空文件 + 持续诱导未来 import 走错路径 |
| 3 | HostSdk 类型搬迁 | **(a) 仅搬 type definition (含 2 个 const)** | Inspector §4 已论证: 实现侧 (`host-sdk-impl.ts`) 依赖 host pinia/i18n/api client, 搬过去会反向耦合, 把 SDK 重新拖回 host 体积; const `HOST_SDK_VERSION` / `HOST_SDK_GLOBAL_KEY` 是纯字面量, 不依赖 host, 一并搬入 |
| 4 | `host-modules.d.ts` 处理 | **(b) 部分清理 (删 `@/types` shim, 保留 vue-virtual + `__APP_CONFIG__`)** | HostSdk 搬入 SDK 后 SDK 内部需自带 `User` 最小 shape, plugin 不再 import `@/types`, 第 18-26 行 shim 删除; SDK DataTable 仍用 `@tanstack/vue-virtual`, plugin 没装该 dep, 第 28-33 行 shim 必须保留; SDK utils/tablePreferences 引用 `window.__APP_CONFIG__`, 第 35-37 行保留 |
| 5 | `GroupPlatform` 类型收缩 | **(a) 接受降级** | 在 SDK 同步 union 类型 = 把业务类型 (`AdminGroup`/`GroupPlatform`) 抽进 SDK, 与 V2 §1.4 决议 "业务类型不入 SDK" 矛盾; 编译期校验放松仅影响 PlatformIcon 一处, 运行时无影响; V2 已接受 PlatformIcon SDK 副本的内联类型, V3 仅延续 |
| 6 | 改造批次 | **(a) 一次性 PR** | 168 处 sed + 7 文件 HostSdk 搬迁是机械改动, 0 风险点 (Inspector §7); 拆批反而增加 host 临时不一致期, 且每批都要 reinstall plugin (§6 摩擦点) |

### 1.1 host-sdk.ts 旧入口处置 — 明确决策

**保留 `frontend/src/plugins/sdk/host-sdk.ts` 改为 1 行 deprecated re-export**:

```ts
// frontend/src/plugins/sdk/host-sdk.ts (V3 改造后)
export * from '@sub2api/plugin-sdk'
```

理由: host 内 4 文件 (`host-sdk-impl.ts` / `host-sdk-window.ts` / `loader-runtime.ts` / `main.ts` 间接) 全部改成 `from '@sub2api/plugin-sdk'`, 但项目根 `frontend/src/plugins/sdk/host-sdk.ts` 的物理路径在文档/外部脚本中可能被 hard-coded; 留 1 行 re-export 作 backstop, 不增加任何运行时成本 (vite tree-shake), 也避免 grep 搜索 `host-sdk.ts` 时让人误以为定义被丢失. 下个版本 (V3.5/V4) 可以再删.

### 1.2 Icon.vue 处置 — 明确决策

**`frontend/src/components/icons/Icon.vue` 保留, 不删**:

理由:
- host 88 处 import 走 `from '@/components/icons/Icon.vue'` (icons 路径, 不是 common 路径)
- 这 88 处 **不在 V3 范围的 168 处之内** (Inspector §2 限定 V3 范围 = `@/components/common/*`)
- Icon.vue host 与 SDK 副本 MD5 SAME (Inspector §1), 留着不会引发漂移
- 删 Icon.vue 等于改 88 处 + plugin host 双源 → 是 V3.5 议题

V3 处理 Icon.vue 的范围**仅限**: 删除 `frontend/src/components/common/` 下没有的 Icon.vue (不存在 — host common 路径下本来就没有 Icon, V2 是从 icons/ 复制到 SDK 的). V3 sed 只处理 8 个 common 组件 + types 的 import.

---

## 2. Implementer 执行单 (高度可抄)

### Step 1 — 补 SDK `index.ts` export `SelectOption` (5 分钟)

`frontend/packages/plugin-sdk/src/index.ts` 在 `export type { Column } from './types'` 一行**之后**追加:

```ts
export type { SelectOption } from './components/Select.vue'
```

(Select.vue 内已 `export interface SelectOption` 见 line 118, 无需搬到 types/index.ts.)

**一次性验证**: `cd frontend && pnpm vue-tsc --noEmit` 通过, 不要求改 host 即可编译过.

### Step 2 — 搬 HostSdk 类型到 SDK 包 (15 分钟)

#### 2.1 复制源文件
```bash
cp frontend/src/plugins/sdk/host-sdk.ts frontend/packages/plugin-sdk/src/host-sdk.ts
```

#### 2.2 改 SDK 副本的 `import type { User } from '@/types'`
SDK 包不能 import host alias. 把第 18 行改成内联最小 shape:
```ts
// frontend/packages/plugin-sdk/src/host-sdk.ts (line 18 替换)
// 原: import type { User } from '@/types'
export interface User {
  id: number
  username: string
  email?: string
  role?: string
  [key: string]: unknown
}
```

#### 2.3 SDK `package.json` 加 peerDeps
```json
"peerDependencies": {
  "vue": "^3.4.0",
  "lucide-vue-next": "^0.300.0",
  "vue-router": "^4.0.0",
  "axios": "^1.0.0"
}
```

#### 2.4 SDK `index.ts` 末尾追加:
```ts
export * from './host-sdk'
```

### Step 3 — 改 host 内 4 处 import (10 分钟)

| 文件 | 改法 |
| --- | --- |
| `frontend/src/plugins/sdk/host-sdk.ts` | **整文件清空**, 改为 `export * from '@sub2api/plugin-sdk'` (deprecated re-export, §1.1) |
| `frontend/src/plugins/sdk/host-sdk-impl.ts` | `from './host-sdk'` 不变 (走 deprecated re-export 即可); **可选优化**: 改成 `from '@sub2api/plugin-sdk'` |
| `frontend/src/plugins/sdk/host-sdk-window.ts` | 同上 |
| `frontend/src/plugins/loader-runtime.ts` | `from './sdk/host-sdk'` 改为 `from '@sub2api/plugin-sdk'` (跨目录, 走包路径更清晰) |
| `frontend/src/plugins/sdk/README.md` | 文档更新, 把 example `import { HostSdk } from './host-sdk'` 改为 `from '@sub2api/plugin-sdk'` |

### Step 4 — 改 plugin 内 2 处 type-only import (5 分钟)

```diff
- // plugins/channel-management/frontend/src/index.ts:20
- import type { HostSdk, PluginRuntimeAssets } from '../../../../frontend/src/plugins/sdk/host-sdk'
+ import type { HostSdk, PluginRuntimeAssets } from '@sub2api/plugin-sdk'

- // plugins/channel-management/frontend/src/api/sdk.ts:12
- import type { HostSdk } from '../../../../../frontend/src/plugins/sdk/host-sdk'
+ import type { HostSdk } from '@sub2api/plugin-sdk'
```

### Step 5 — 改 host 168 处 import 走 SDK (20 分钟, sed 主导)

#### 5.1 default import → named import (8 个组件, ~152 处)

```bash
# DataTable / BaseDialog / Select / Pagination / ConfirmDialog / EmptyState / Toggle / PlatformIcon
for COMP in DataTable BaseDialog Select Pagination ConfirmDialog EmptyState Toggle PlatformIcon; do
  find frontend/src \( -name '*.vue' -o -name '*.ts' \) -print0 | \
    xargs -0 sed -i -E "s|^([[:space:]]*)import ${COMP} from '@/components/common/${COMP}\.vue'|\1import { ${COMP} } from '@sub2api/plugin-sdk'|g"
done
```

**注意**: 上述 sed 仅匹配行首是 import 的形式 (`^([[:space:]]*)import`), 不会误匹配注释/字符串. 但 `Inspector §2` 提到有 2 处出现 `import AppLayout from '...'; import Pagination from '...'` 的同行连写, sed 行级匹配会漏掉第二个 — 改造后用 `grep -rn "from '@/components/common/(DataTable|BaseDialog|Select|Pagination|ConfirmDialog|EmptyState|Toggle|PlatformIcon)\.vue'"` 复核, 手工补漏.

#### 5.2 type-only import (Column 14 处 + SelectOption 2 处)

```bash
# Column (host 现在 from './common/types' 或 from '@/components/common/types')
find frontend/src \( -name '*.vue' -o -name '*.ts' \) -print0 | \
  xargs -0 sed -i -E "s|import type \{ Column \} from '@/components/common/types'|import type { Column } from '@sub2api/plugin-sdk'|g"

# SelectOption (2 处: PaymentProviderDialog.vue:212, AccountsView.vue:338)
find frontend/src \( -name '*.vue' -o -name '*.ts' \) -print0 | \
  xargs -0 sed -i -E "s|import type \{ SelectOption \} from '@/components/common/Select\.vue'|import type { SelectOption } from '@sub2api/plugin-sdk'|g"
```

#### 5.3 命名 import 形态 (混合 default + type)
若文件中存在 `import Select, { type SelectOption } from '@/components/common/Select.vue'` 这种混合形式, sed 不能可靠处理. **改造前**先执行检查:
```bash
grep -rEn "import [A-Z][A-Za-z]*,[[:space:]]*\{" frontend/src --include='*.vue' --include='*.ts' | \
  grep '@/components/common/'
```
若有命中, **手工**改为单 named import 形式后再走 sed.

#### 5.4 Icon 不动
host 88 处 `from '@/components/icons/Icon.vue'` **保持原样** (§1.2). V3 不动 icons/ 路径.

### Step 6 — 删除 host common 副本 (5 分钟)

```bash
rm frontend/src/components/common/DataTable.vue
rm frontend/src/components/common/BaseDialog.vue
rm frontend/src/components/common/Select.vue
rm frontend/src/components/common/Pagination.vue
rm frontend/src/components/common/ConfirmDialog.vue
rm frontend/src/components/common/EmptyState.vue
rm frontend/src/components/common/Toggle.vue
rm frontend/src/components/common/PlatformIcon.vue
rm frontend/src/components/common/types.ts
# Icon.vue 留在 frontend/src/components/icons/, 见 §1.2
# tablePreferences.ts 留在 frontend/src/utils/, 因为 host 自己也用
```

### Step 7 — 清 plugin `host-modules.d.ts` 中 `@/types` shim (3 分钟)

`plugins/channel-management/frontend/src/host-modules.d.ts` 删除第 18-26 行 (`declare module '@/types'` 整段), 保留 28-33 行 (`@tanstack/vue-virtual`) 与 35-37 行 (`Window.__APP_CONFIG__`). 同时改头部注释, 删去 "1. host-sdk.ts (HostSdk) 内部 import type { User } from '@/types' ..." 那段说明.

### Step 8 — 验收 (按 §3 清单逐项)

```bash
# Backend (兜底, V3 不改后端)
cd backend && go build ./...

# Frontend
cd frontend
pnpm install                  # 重新链接 workspace
pnpm vue-tsc --noEmit         # 168 处 import + HostSdk 搬迁后类型必须通过
pnpm build                    # 产物大小对比

# Plugin (注意 file: 协议必须 reinstall)
cd ../plugins/channel-management/frontend
pnpm install                  # ⚠️ 必须 (Inspector §6: file: 协议会重新解析 + copy)
pnpm build                    # 产物大小应 ≈ V2 baseline ~76KB
```

---

## 3. 验收清单

- [ ] `pnpm vue-tsc --noEmit` 在 host frontend 通过 (168 处替换全部 type-clean)
- [ ] `pnpm vue-tsc --noEmit` 在 plugin channel-management 通过 (HostSdk 走 SDK 包)
- [ ] `pnpm build` 在 host frontend 成功, host bundle 体积**减少** (8 个 .vue + types.ts 不再被 host 主 bundle 直接打包, 改为通过 workspace 软链入 SDK source, vite tree-shake 应保持稳定或微减)
- [ ] `pnpm build` 在 plugin 成功, dist/entry.js ≈ 76KB (V2 baseline, 不应回升)
- [ ] `grep -rn "from '@/components/common/\(DataTable\|BaseDialog\|Select\|Pagination\|ConfirmDialog\|EmptyState\|Toggle\|PlatformIcon\|types\)" frontend/src` 命中 = 0 行
- [ ] `grep -rn "from '../../../frontend" plugins/channel-management/frontend/src/` 命中 = 0 行
- [ ] `grep -rn "from '../../../../frontend" plugins/channel-management/frontend/src/` 命中 = 0 行
- [ ] `frontend/src/components/common/` 下 8 个 .vue + types.ts 已删除 (Icon 不在此目录, 保留 icons/Icon.vue)
- [ ] `frontend/packages/plugin-sdk/src/host-sdk.ts` 存在并 export `HostSdk` interface + `HOST_SDK_VERSION` const + `HOST_SDK_GLOBAL_KEY` const
- [ ] `frontend/packages/plugin-sdk/package.json` peerDependencies 含 `vue-router` + `axios`
- [ ] 浏览器手动验证: 打开 `/admin/channels`, DataTable / Pagination / Select / ConfirmDialog 行为不变; 打开 host `/admin/users` (DataTable 主战场) 翻页/搜索/排序正常

---

## 4. 风险点

1. **混合 import 形态漏改 (§5.3)**: `import Select, { type SelectOption } from ...` 这种混合 default+type 形式 sed 不能处理, 必须先手工拆分. 提交前必须 grep 复核.
2. **plugin file: 协议 reinstall (Inspector §6)**: SDK 类型变更后 (新增 `host-sdk` export), plugin 不 `pnpm install` 就编译时会拿到旧的 copy. 改造文档需明确写 "Step 8 必须先 reinstall, 不能跳过".
3. **deprecated re-export 引导力 (§1.1)**: `frontend/src/plugins/sdk/host-sdk.ts` 留 1 行 re-export 是 backstop, 但可能让人误以为 "host-sdk 仍是 host 内定义" — 在文件头加 1 行注释 `// DEPRECATED: moved to @sub2api/plugin-sdk in V3, kept as re-export for backward compat`.
4. **GroupPlatform 类型校验放松 (议题 #5)**: PlatformIcon 在 SDK 内 `type GroupPlatform = string`, host 调用方传错平台名也不会报编译错. 测试时手工传一个不存在的 platform (如 `"xxx"`) 验证 fallback 渲染 (应是空/默认 icon, 不该 crash).
5. **vite-tsconfig-paths 与 workspace 解析冲突**: host 当前 `tsconfig.json` `paths` 把 `@/*` 指到 `src/*`, SDK 包内的 `host-sdk.ts` 不应该走 host alias. SDK 副本的 `User` 已内联化 (Step 2.2 已处理), 但 vite-tsconfig-paths 若激进解析可能在 SDK 内继续 resolve `@/types`. 编译失败时检查 SDK 包的 `tsconfig.json` (若有) 是否独立于 host paths.

---

## 5. 不在范围内 (V3 不做)

- **host-only 组件 SDK 化**: LoadingSpinner / GroupBadge / HelpTooltip / GroupOptionItem / SearchInput / DateRangePicker / ProxySelector / LocaleSwitcher / ImageUpload / Input / ModelIcon / NavigationProgress / ExportProgressDialog / GroupSelector / AnnouncementBell / AnnouncementPopup 等 25 个组件, 78 处 host 内 import — 全是 host 内单向使用, plugin 不依赖, 抽 SDK 无收益. 是 V3.5 / V4 议题.
- **业务类型 SDK 化**: `AdminGroup` / `GroupPlatform` (union) / `User` 完整定义抽进 SDK — V2 §1.4 已砍, V3 延续. plugin 自定义最小 shape 即可.
- **`@/components/icons/Icon.vue` 路径迁出**: host 88 处 hard-coded 该路径, 不在 V3 的 168 处机械范围内. 需要 `Icon.vue` 删除等于额外 88 处 sed + 重新评估 host icons/ 目录定位 — 是 V3.5 议题.
- **全局样式 / token / 后端能力 SDK 化**: tailwind config / CSS variables / Go SDK 都不在前端 V3 范围. V4 议题.
- **HostSdk 实现侧搬迁**: `host-sdk-impl.ts` / `host-sdk-window.ts` / `expose-runtime.ts` 全部留在 host, 因为依赖 host pinia/i18n/api client. 搬过去 = 反向耦合.
- **plugin vite.config.ts 清理**: V2 已删除 alias `@` 和 `hostNodeModulesResolver` 等折中, V3 不再变更.

---

## 6. 给 Implementer 的开工前 Sanity Check (1-2 项, 必做)

1. **workspace pnpm install 干净**: 在 `frontend/` 跑 `pnpm install`, 必须看到 `@sub2api/plugin-sdk` 通过 `workspace:*` 软链 (`node_modules/@sub2api/plugin-sdk` 是 symlink → `frontend/packages/plugin-sdk`). 若不是 symlink, 检查 `frontend/pnpm-workspace.yaml` 是否存在并包含 `'packages/*'` 一行; 不要硬上 sed 改造, 否则 host 编译时会找不到 SDK.
2. **plugin vue-tsc 基线**: 在改任何代码前, 先 `cd plugins/channel-management/frontend && pnpm install && pnpm vue-tsc --noEmit`, 记录当前是否已无报错. V2 完成后此命令应该是 0 错误; 若已经有错, **先停下来调查**, 不要把 V3 改动叠加在已破损的基线上.
