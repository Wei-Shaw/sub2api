# Design: Custom Menu Red Dot

## Context

项目已有 `recharge-bonus` capability 提供的红点机制作为原型。本次需要把同一套模式**移植**到自定义菜单，但要处理三处非平凡的差异：

1. 自定义菜单是**多项数组**，充值活动是**单一全局配置**——红点粒度可选：全局 / 每项独立。
2. 自定义菜单目前**没有 version 概念**——需要新加。
3. admin 的语义诉求：不是"只要有 items 就出红点"，而是"我明确希望这次投放拉一次注意"——需要一个可开关的旋钮。

## Decisions

### 决策 1：红点粒度 = 全局单点

**采用**：整个自定义菜单区共享一个 `version`，一个 dismiss key。用户点击**任意一个自定义菜单项**（或直接访问其 `/custom/:id`）都会 dismiss 全部红点。

**备选**：每 item 独立 hash，独立 dismiss key（`custom-menu-seen:<userId>:<itemId>:<itemHash>`）。

**理由**：
- 用户视角：多个红点同时闪烁是噪音；一次 dismiss 全清更符合"我知道菜单更新了"这一心智。
- 运营视角：admin 通常一次改一批（换主题 / 上新活动季），期望是"这次的更新用户看过就好"，而不是"用户必须点完每一个"。
- 实现视角：全局粒度只需要一个 hash 输入（整个规范化 items JSON）、一个 storage key、一个 composable，代码量约为 per-item 方案的 1/3。
- 若未来出现"必须逐项标记"的诉求，可以增量升级（key 加 `<itemId>`），届时旧 dismiss 记录仍能被 `<userId>:<version>` 前缀识别为过期。

### 决策 2：version 生成时机 = 保存时算 + 缓存

**采用**：`ComputeCustomMenuVersion(rawJSON) -> hex12` 是**纯函数**。写路径（admin 保存）计算并落库到 `SettingKeyCustomMenuVersionCache`；读路径直接取缓存值。**不**在读取时懒算。

**备选**：读时懒算（每次 GET public settings 现算 hash）。

**理由**：
- public settings 是高频端点（每次前端启动 + 页面切换），懒算导致每次 SHA-256 一次 20-item 的 JSON，无必要的 CPU 消耗。
- 保存路径已经有 `checkCustomMenuChanged` 的 diff 逻辑（见 `admin/setting_handler.go:2849`），插入 hash 计算成本极低。
- **关键副作用要处理**：`custom_menu_red_dot_enabled` 从 `false → true` 的切换本身**不改变 items JSON**，但语义上应触发新周期（否则开关打开还是老 hash，用户看不到红点）。
  - 解决：hash 输入除了规范化 items JSON，还要 include `enabled` 字段本身。开关切换即改变 hash 输入 → 新 version。
  - `enabled: true → false` 切换也会改 hash，但此时前端不显示（`shouldShow` 短路），无副作用；下次 `false → true` 又是新 version（因为再次翻转 → 新的复合 hash）。
  - **实际实现**：hash input = `sha256({"enabled": <bool>, "items": <normalized items JSON>})`。

### 决策 3：dismiss 触发点 = 3 处

对齐用户"三选一都算"的确认：

| 触发点 | 位置 | 触发时机 |
|---|---|---|
| 侧栏点击（桌面） | `AppSidebar.handleMenuItemClick` | 用户点击任意 `customMenuItemsForUser` item |
| Drawer 点击（移动） | 同上（同一函数） | 同上 |
| iframe 页直达 | `CustomPageView.onMounted` | 路由 `/custom/:id` 加载完成 |

`same_tab` / `new_tab` action 类型在点击时也走 `handleMenuItemClick`（见 AppSidebar 现有实现），所以三个 action 变体在点击层面一视同仁——统一在 `handleMenuItemClick` 里加一行 `customMenuDot.dismiss()` 即可。

### 决策 4：红点样式 = 对齐充值 tab

复用 `recharge-bonus` 已有的 red dot CSS 类（在 `PaymentView.vue` / `RechargeTabButton` 里）。若样式散在组件里而非 utility class，本次抽出一个 `<RedDotBadge />` 极简组件（`w-2 h-2 rounded-full bg-red-500 absolute top-1 right-1`），充值和自定义菜单同用；否则直接复用 tailwind 组合。**具体路径待实现时看代码，不硬约束在 spec 里。**

### 决策 5：admin 只读展示 version

admin 面板在开关旁显示**当前 hash 短值**（如 `a3f5b2c9d1e0`）+ 说明。不显示"上次生成时间"字段——因为 hash 是纯派生，最后一次改动的时间可以用现有 `settings.updated_at` 近似（同一个 admin 保存动作里）。若 admin 强调需要精确到"上次开关翻转时间"，另开子任务。

## Data Flow

```
Admin edits custom menu / toggles switch
        │
        ▼
POST /api/admin/settings
        │
        ▼
setting_handler.UpdateSettings
   ├─ persist custom_menu_items JSON
   ├─ persist custom_menu_red_dot_enabled bool
   └─ compute new_version = ComputeCustomMenuVersion({enabled, items})
        │
        ▼
Store SettingKeyCustomMenuVersionCache = new_version
        │
        ▼
GET /api/v1/settings/public
        │
        ▼
{ custom_menu_items:[...], custom_menu_red_dot_enabled: true, custom_menu_version: "a3f5..." }
        │
        ▼
appStore.cachedPublicSettings
        │
        ├─────────────► AppSidebar
        │                    │
        │                    ▼
        │              useCustomMenuRedDot(userId)
        │                    │
        │              shouldShow = enabled && version && !localStorage[`custom-menu-seen:${userId}:${version}`]
        │                    │
        │              [User clicks any custom menu item]
        │                    │
        │                    ▼
        │              dismiss() → localStorage.setItem(...) + bumpSharedTick()
        │                    │
        │                    ▼ (via storage event / sharedDismissTick)
        │              AppSidebar 与 CustomPageView 的 shouldShow 均转 false
        │
        └─────────────► CustomPageView
                             │
                       onMounted → dismiss()
```

## Alternatives Considered

- **不用 hash，用 `updated_at` 戳**：简单但每次 admin 保存都会刷红点（即使 no-op）——违背用户明确说的"避免打扰"意图，pass。
- **红点里带数字（如 "3"）表示新增的 item 数**：需要额外记录每个用户上次看到时的 item 集合，复杂度陡增。默认单点更符合 KISS，可作为后续增量。
- **每个 item 一个开关**（`show_red_dot: bool` 落在 item 结构里）：给 admin 更细粒度控制，但 UI 成本 × N，且违背"决策 1 全局粒度"。若确有需要，未来可以在 item 上加 `boost_priority: bool` 之类做二级筛选，不阻塞本次交付。

## Open Questions

- Q：iframe 内嵌页里，用户在页面停留 5 秒后再看到"新内容"感觉才更强，是否要等 iframe 加载完再 dismiss，而不是路由挂载即 dismiss？
  A（暂定）：路由挂载即 dismiss。用户主动进入这个页面已经体现了"我知道有这个入口"的信号，等 iframe 加载失败也不该反复出红点。
- Q：admin 是否需要"手动重置红点周期"按钮（等价于强行 mint 新 version）？
  A（暂定）：不需要。任何字段改动都会 mint 新 version；如果 admin 真的什么都不想改但要"重出红点"，可以 toggle enabled 一下（false→保存→true→保存），间接达成。若这成为高频操作，再加显式按钮。
