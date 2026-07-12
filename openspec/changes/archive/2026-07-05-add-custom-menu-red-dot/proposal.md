## Why

Operators使用自定义菜单（`custom_menu_items`）向用户投放外部业务入口（游戏、社区、活动页等）。当前的痛点是：**用户看不见新菜单**——菜单项加进侧边栏后没有任何视觉提示，用户不会主动点开一个已经"看熟"的侧栏。运营方希望在**新增/修改菜单项**后能强制吸引一次注意，看过之后自然消失，直到下次真正有变化再触发下一轮。

这与已实现的 `recharge-bonus` 红点是同一模式（后端算 `version`，前端 `localStorage` 落 `<userId>:<version>`，一次 dismiss 永久生效直到 version 变化），本次是把该模式复用到自定义菜单上，但保留一个关键差异：**红点由 admin 显式开关控制**，而不是"只要有 items 就必然出红点"——避免每次微调 label / icon 都无差别打扰用户。

## What Changes

### Backend
- 新增 setting `CUSTOM_MENU_RED_DOT_ENABLED`（bool，默认 `false`）——admin 显式开关，off 时前端一律不显示红点。
- 新增派生字段 `custom_menu_version`（string），由后端在 `custom_menu_items` **规范化 JSON** 上取 SHA-256 前 12 hex chars 计算：
  - 规范化包含：按 `sort_order` 排序 → 每项内按 key 字母序 emit → 去除空白 → 只包含展示字段（`id / label / icon_svg / url / page_slug / action / visibility / sort_order`）。
  - **admin 无差异保存 → hash 相同 → version 不变 → 已 dismiss 的用户不刷红点**（对齐 `recharge-bonus` 语义）。
  - 计算入口：`service.ComputeCustomMenuVersion(rawJSON string) string`；缓存在 `settings_view` 的读路径上，避免每次 GET 重算。
- Public settings response（`GET /api/v1/settings/public` / 或现有 cachedPublicSettings 端点）新增两个字段：
  - `custom_menu_red_dot_enabled: bool`
  - `custom_menu_version: string`
- Admin `SystemSettings` DTO 同步新增上述两个字段（前者可写，后者只读）。
- `SettingsView` 变更审计新增 `custom_menu_red_dot_enabled` 键。

### Frontend
- **Admin · 系统设置 → 自定义菜单区块**：在现有 `custom_menu_items` 编辑器上方新增一个开关块：
  - Toggle：`展示红点提醒`（绑定 `custom_menu_red_dot_enabled`）。
  - 说明文字：'开关 off→on 或菜单内容变化时会生成新的红点周期，已看过的用户会重新看到红点。'
  - 显示当前 `custom_menu_version` 值（monospace 短 hash + 复制按钮），以及生成时间（复用 `updated_at` 或新增 `custom_menu_version_minted_at`——见 design.md 决策）。
- **`AppSidebar.vue`**：
  - 每个 `customMenuItemsForUser` 生成的 nav item 引入红点渲染（对齐充值 tab 的红点样式：右上角 `w-2 h-2` 圆点 + 默认色 token）。
  - 桌面态与移动 drawer 复用同一 DOM，无需分别处理。
  - 点击菜单项时（`handleMenuItemClick` 内）触发 dismiss。
- **`CustomPageView.vue`**（iframe 类型菜单页）：在 `onMounted` / route resolve 时也触发 dismiss——覆盖"用户从 URL 直接进入 `/custom/:id` 而没经过侧栏点击"的情况。
- 新增 composable `useCustomMenuRedDot(userId)`：
  - 复用 `useRechargePromoDot` 的 dismiss 语义（模块级 `sharedDismissTick` + `storage` 事件跨 tab 同步）。
  - localStorage key: `custom-menu-seen:<userId>:<version>`（**全局粒度**，非 per-item——本次决定；见 design.md 差异 1）。
  - 读入口：`shouldShow: ComputedRef<boolean>`，`dismiss(): void`。
- i18n 新增键：`admin.settings.customMenu.redDot.*` / `nav.customMenu.newBadgeAria` 等，中英双语。

### Tests
- Backend: `ComputeCustomMenuVersion` 稳定性（相同 JSON、字段乱序输入 → 相同 hash）、字段变更触发 version 变化、opt-out 字段（如 `updated_at`）不影响 hash；public settings 端点带出新字段。
- Frontend: `useCustomMenuRedDot` 显示逻辑（enabled=false / userId=null / 已 dismiss 三种情况均不显示）、点击侧栏 item 触发 dismiss、跨 tab storage 事件生效、`CustomPageView` 挂载触发 dismiss。

## Capabilities

### New Capabilities
- `custom-menu-red-dot`：为 `custom_menu_items` 提供 admin 可开关的红点提醒机制，包含后端 version 派生、public settings 暴露、前端 sidebar / drawer / 直达页三处的 dismiss 触发点，以及跨 tab 同步的一次性 dismiss 语义。

### Modified Capabilities
- `console-navigation`：`AppSidebar` 的自定义菜单项渲染新增红点插槽——现有 brand block / 折叠态 / 键盘可达性 requirement 不变，但需追加一条关于自定义菜单红点的 requirement 及 scenario。

## Impact

- **Schema migration**：无。仅新增两个 settings key（一个可写 bool，一个派生字符串）。默认 `enabled = false` 保证升级即 no-op。
- **API surface**：
  - `GET /api/v1/settings/public`：additive，新增两字段。旧客户端忽略即可。
  - `POST /api/admin/settings`：接受 `custom_menu_red_dot_enabled`；无值时保持现状。
- **Affected code**：
  - Backend: `internal/service/domain_constants.go`（新增 setting key）、`internal/service/settings_view.go`（补 struct 字段 + version 计算）、`internal/service/setting_service.go`（保存路径）、`internal/handler/dto/settings.go`（DTO 补字段）、`internal/handler/setting_handler.go` & `admin/setting_handler.go`（响应装配 + audit）。
  - Frontend: `components/layout/AppSidebar.vue`、`views/user/CustomPageView.vue`、`views/admin/SettingsView.vue`（自定义菜单区块 UI）、`composables/useCustomMenuRedDot.ts`（新）、`stores/appStore` cachedPublicSettings 类型定义、i18n `zh-CN.ts` / `en.ts`。
- **No external dependencies** added. **No breaking changes**：`custom_menu_red_dot_enabled` 默认 `false`，升级后所有用户看到的效果与今天完全一致，直到 admin 主动打开开关。
