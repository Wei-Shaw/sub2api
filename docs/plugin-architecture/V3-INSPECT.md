# V3 Inspector 残余耦合审计

> 角色: Inspector (read-only)  •  基线 `feat/plugin-system-fixes` (`7eb03e79` Merge SDK-V2 implementer)
> 子分支 `feat/plugin-system-fixes--v3-inspect`  •  所有路径相对仓库根

---

## 1. host common 副本与 SDK 字节级 diff

V2 Implementer 报告 "byte-identical copy" 的描述 **不准确**。SDK 包内 9 个 .vue + types + 1 个 utils 与 host 对比如下：

| 文件 | host MD5 | SDK MD5 | 状态 | 唯一差异 |
| --- | --- | --- | --- | --- |
| Icon.vue | 909753e3… | 909753e3… | SAME | — |
| ConfirmDialog.vue | 000e0825… | 000e0825… | SAME | — |
| Toggle.vue | 5aec3cd8… | 5aec3cd8… | SAME | — |
| BaseDialog.vue | db91b5f2 | 0c082a85 | DIFF | `@/components/icons/Icon.vue` → `./Icon.vue` |
| EmptyState.vue | 691efb2d | cbedca21 | DIFF | 同上 (Icon 路径) |
| Select.vue | b64573ce | 942ab1be | DIFF | 同上 |
| DataTable.vue | 3d37a771 | f93ca334 | DIFF | Icon 路径 + `./types` → `../types` |
| Pagination.vue | 404ee259 | 05bbb2cf | DIFF | Icon 路径 + `@/utils/tablePreferences` → `../utils/tablePreferences` |
| PlatformIcon.vue | 841d31cb | c5b060e8 | DIFF | host `import type { GroupPlatform } from '@/types'` → SDK 内联 `type GroupPlatform = string` |
| utils/tablePreferences.ts | 1280f815 | 1280f815 | SAME | — |
| types (host=`common/types.ts` vs SDK=`types/index.ts`) | — | — | 内容字节相同 (diff 0 行, 各 11 行) | 仅文件路径不同 |

**结论**: 6 处 DIFF 全部是为了打破 `@/` host alias 而做的相对路径改写 + 1 处类型内联 (PlatformIcon 的 `GroupPlatform` 收缩为 `string`)；语义上仍是同一份组件源码。**没有任何业务逻辑/模板/样式漂移**, V3 直接以 SDK 包为单一来源、删除 host 副本是安全的, 不需要先做 "host/SDK 调和"。

PlatformIcon 的类型收缩 (`GroupPlatform = string`) 是实际语义降级 —— host 原来是字符串 union, SDK 弱化为任意字符串。host 视图改走 SDK 后, 编译期类型校验会变松。Curator 可决定: 接受 / 在 SDK types 里同步 union 定义 (但牵涉业务类型, 与 #3 同源, V3 文档声明暂不处理)。

---

## 2. host 235 处 import 实际是 246 处

`grep -rE "@/components/common" frontend/src --include='*.vue' --include='*.ts'` 真实计数：

- **246 处 import** 分布在 **93 个文件**
- 0 处 dynamic import (`import(...)`), 0 处 `defineAsyncComponent`
- 0 处来自测试文件 (`*.spec.ts` / `__tests__/`)
- 16 处是 type-only import (14× `Column`, 2× `SelectOption`)

**按被引用组件分组 (top)**：

| 组件 | 出现次数 | 是否在 SDK |
| --- | --- | --- |
| BaseDialog | 51 | ✅ |
| Select | 41 | ✅ |
| Pagination | 20 | ✅ |
| ConfirmDialog | 20 | ✅ |
| EmptyState | 15 | ✅ |
| DataTable | 15 | ✅ |
| types (`Column`) | 14 | ✅ (SDK `./types` re-exported) |
| LoadingSpinner | 13 | ❌ host-only |
| GroupBadge | 8 | ❌ |
| HelpTooltip | 7 | ❌ |
| GroupOptionItem | 5 | ❌ |
| Toggle / GroupSelector / DateRangePicker | 4 each | Toggle ✅, 余 ❌ |
| PlatformIcon / ProxySelector / LocaleSwitcher | 3 each | PlatformIcon ✅, 余 ❌ |
| 其余 17 个组件 | 1-2 each | 全部 host-only |

**SDK 已覆盖的 8 个组件 + types** 总计 **168 处 import 跨 76 个文件**, 这是 V3 真正可机械改造的工作量。剩 **78 处 / 25 个 host-only 组件** 仍走 `@/components/common/*`, 不在 V3 范围内。

**按 host 文件分组的密度 top-10 (全部走机械改动即可)**：

```
KeysView.vue:10  SubscriptionsView.vue:9  GroupsView.vue:9  ProxiesView.vue:8
UsersView.vue:7  RedeemView.vue:7  AnnouncementsView.vue:7  UsageView.vue (user):6
AdminPaymentPlansView.vue:6  SettingsView.vue:6
```

机械改动评估: **168/168 sed 可改 (100%)**。两个特殊形式都是规则的：
- `import Foo from '@/components/common/Foo.vue'` → `import { Foo } from '@sub2api/plugin-sdk'`
- `import type { Column } from '@/components/common/types'` → `import type { Column } from '@sub2api/plugin-sdk'`
- `import type { SelectOption } from '@/components/common/Select.vue'` → 需要先在 SDK `index.ts` 里 `export type { SelectOption } from './components/Select.vue'` (当前 SDK 仅暴露 `Column`)

**Curator 必读**: SDK `index.ts` 现状只 re-export `Column`, **`SelectOption` 没暴露**。V3 必须先补 SDK export 再做 host import 替换, 否则 2 处 (`PaymentProviderDialog.vue:212`, `AccountsView.vue:338`) 会编译失败。

---

## 3. plugin type-only 跨目录引用清单

**确切定位 2 处 (V2 Implementer 报告吻合)**：

```
plugins/channel-management/frontend/src/index.ts:20
  import type { HostSdk, PluginRuntimeAssets } from '../../../../frontend/src/plugins/sdk/host-sdk'

plugins/channel-management/frontend/src/api/sdk.ts:12
  import type { HostSdk } from '../../../../../frontend/src/plugins/sdk/host-sdk'
```

引用的全部是 V1 在 `frontend/src/plugins/sdk/host-sdk.ts` 定义的 type-only 契约 (`HostSdk` interface 及其子接口 `HostTheme/HostFont/HostI18n/HostNotify/HostRouter/HostAuth/HostHttp/HostVue` + `PluginRuntimeAssets/PluginRuntimeModule`)。运行时无副作用 (vite externals + importmap 处理)。

`from '@/'` 在 plugin src 里 grep 命中 **0 处** (V2 Implementer 描述准确, 仅 host-modules.d.ts 注释里提到)。 `host-modules.d.ts` 仍存在并 declare 了 3 个 type shim: `@/types` (User), `@tanstack/vue-virtual`, `Window.__APP_CONFIG__`。

**这些类型是否需要从 host 拉**: 是。`HostSdk` 是 host↔plugin 的契约总线, 不可在 plugin 自写最小契约 (会 silently drift)。最佳处置: 把 `host-sdk.ts` 整体 (仅 type 定义部分) 搬到 SDK 包, plugin import `from '@sub2api/plugin-sdk'`, host 也 import 自同一包 —— 单一来源。

---

## 4. HostSdk 搬迁影响面

`HostSdk` 类型搬到 `frontend/packages/plugin-sdk/` 后影响清单：

**host 侧需改 import (5 个文件)**:
- `frontend/src/plugins/sdk/host-sdk-impl.ts` — 实现侧, `import { ..., type HostSdk } from './host-sdk'` 改为从 SDK 包
- `frontend/src/plugins/sdk/host-sdk-window.ts` — `import { HOST_SDK_GLOBAL_KEY, type HostSdk } from './host-sdk'`
- `frontend/src/plugins/loader-runtime.ts` — `import type { HostSdk, PluginRuntimeAssets, PluginRuntimeModule } from './sdk/host-sdk'`
- `frontend/src/main.ts` — `import { attachHostSdkToWindow } from '@/plugins/sdk/host-sdk-window'` (无需改, 因为 host-sdk-window 是实现, 留在 host)
- `frontend/src/plugins/sdk/README.md` — 文档更新

**plugin 侧改 import (2 个文件)**:
- `plugins/channel-management/frontend/src/index.ts:20`
- `plugins/channel-management/frontend/src/api/sdk.ts:12`
- 以及 `host-modules.d.ts` 中 `declare module '@/types' { User }` shim 可删除 (因为 `HostSdk` 不再 import `@/types`, 应在搬迁时把 `User` 类型也内联或在 SDK 自定义最小 shape)

**SDK package.json 需要新增的 peerDependencies**:
- `vue-router` (`HostRouter` 用 `Router`, `RouteLocationNormalizedLoaded`, `RouteRecordRaw`)
- `axios` (`HostHttp.apiClient: AxiosInstance`)
- `pinia` 不必加 (HostSdk 接口不直接 ref pinia, 实现侧 `host-sdk-impl.ts` 才用)
- `vue-i18n` 不必加 (HostI18n 自定义类型, 不引 vue-i18n 的 namespace)
- `vue` 已经在 peerDeps 里

**搬迁分支选择 (建议)**: **只搬 type definition** (`host-sdk.ts` 中 `interface/type/const`), **保留 `host-sdk-impl.ts` / `host-sdk-window.ts` / `expose-runtime.ts` 在 host**。理由: 实现侧依赖 host pinia store (`useAppStore`/`useAuthStore`), `@/i18n`, `@/api/client`, `@/stores/*` 这些都是 host-internal, 搬到 SDK 包会反向耦合, 把 SDK 重新拖回 host 体积 —— 违背 V2 解耦的初衷。

---

## 5. AppLayout 等 V2 砍掉项的现状

`grep AppLayout plugins/` 结果 **2 处, 均为注释**：

```
ChannelsView.vue:2     <!-- V2 SDK 改造: AppLayout 删除 (plugin 已在 host PluginView 内...
ChannelsView.vue:1096  /* 注意 plugin 已在 PluginView 内, AppLayout 已套, ... */
```

无 `import` 语句残留。V2 Curator §1 表格的"AppLayout 误用"已落实, V3 无残留任务。

---

## 6. workspace + plugin file: 协议现状

**`frontend/pnpm-workspace.yaml`** (V2 新建):
```yaml
packages:
  - '.'              # host frontend (root)
  - 'packages/*'     # @sub2api/plugin-sdk
```

**`frontend/package.json`** ref: `"@sub2api/plugin-sdk": "workspace:*"` (走 pnpm workspace 软链)

**`plugins/channel-management/frontend/package.json`** ref: `"@sub2api/plugin-sdk": "file:../../../frontend/packages/plugin-sdk"` —— **不在 host workspace 内**, 走 npm `file:` 协议 (因 plugin 顶层 `plugins/channel-management/` 是独立 npm 模块, 而非 frontend 的子 workspace)。

**V3 影响评估**:
- V3 把 host 246 处 import 改走 SDK 包 → workspace 软链已就绪, 不需新增配置
- V3 把 `host-sdk.ts` 类型搬入 SDK 包 → host `loader-runtime.ts` / `host-sdk-impl.ts` 改成 `import from '@sub2api/plugin-sdk'`, 走 workspace, 同样不需新增配置
- plugin 侧 `file:` 协议会重新解析 (npm install 时 copy 节点), 类型变更需要 plugin 那边 `npm i` 重装 —— **是 V3 落地的隐藏摩擦点**, Curator 应在 PLAN 里写明 "搬迁后 plugin frontend 必须 reinstall"

无破坏性影响。

---

## 7. 结论：V3 是"大而机械"

| 维度 | 评估 |
| --- | --- |
| host 副本与 SDK 是否字节相同 | **否**, 但 6 处 DIFF 全是路径改写 + 1 处类型收缩, 无业务漂移; 直接删除 host 副本安全 |
| host 246 处 import 中机械改动比例 | **168/168 = 100%** 可走 sed (限定 8 个 SDK 组件 + types); 余 78 处 host-only 不在 V3 范围 |
| HostSdk 搬迁 "血量" | host 4 文件 (`host-sdk-impl/-window/loader-runtime/README`) + plugin 3 文件 (`index.ts/api/sdk.ts/host-modules.d.ts`); 仅搬类型不搬实现; SDK package.json 增加 `vue-router` + `axios` peerDeps |
| 隐藏摩擦点 | (a) `SelectOption` SDK 未暴露, V3 第一步必须补 export; (b) `GroupPlatform` 在 PlatformIcon 中已类型收缩, host 改走 SDK 后会丢 union 校验; (c) plugin 走 `file:` 协议, 搬迁后须 reinstall |
| 风险 | 低; 无 dynamic import / 无测试 import / 无 host-sdk 实现搬迁 |

V3 改造形态: **大而机械** (168 处 sed-able replace + 7 文件 HostSdk 类型搬迁), 不是 "小而琐碎"。Curator 可以放心给 Implementer 一份高度脚本化的执行单。
