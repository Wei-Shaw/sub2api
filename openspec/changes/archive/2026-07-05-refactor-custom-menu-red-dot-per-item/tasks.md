## 1. Backend

- [x] `dto.CustomMenuItem` 新增 `ShowRedDot bool json:"show_red_dot,omitempty"`。
- [x] `dto.SystemSettings` / `dto.PublicSettings` 删除 `CustomMenuRedDotEnabled bool` 字段。
- [x] `service.ComputeCustomMenuVersion` 签名改为 `func(itemsJSON string) string`，payload 移除 `enabled`；`show_red_dot` 有意不进入 hash。
- [x] `service.SettingService`（`domain_constants.go` / `settings_view.go` / `setting_service.go`）删除 `SettingKeyCustomMenuRedDotEnabled` 常量、`CustomMenuRedDotEnabled` 字段、defaults / read / write / audit diff 相关代码；`SettingKeyCustomMenuVersionCache` 的写入改用单参数版本。
- [x] `handler.SettingHandler` 与 `handler/admin.SettingHandler` 删除对 `CustomMenuRedDotEnabled` 的映射与 `UpdateSystemSettingsRequest.CustomMenuRedDotEnabled *bool`。
- [x] 更新 `custom_menu_version_test.go`：删除 `TestComputeCustomMenuVersion_EnabledToggleMatters`，新增 `TestComputeCustomMenuVersion_ShowRedDotToggleIgnored` 断言 show_red_dot 不影响 hash；其他单参数化。
- [x] `go build ./... && go test ./internal/service/... ./internal/handler/...` 全绿。

## 2. Frontend

- [x] `types.CustomMenuItem` 新增 `show_red_dot?: boolean`；`types.PublicSettings` 删除 `custom_menu_red_dot_enabled`。
- [x] `api/admin/settings.ts` 删除 `custom_menu_red_dot_enabled`（response + request）。
- [x] `stores/app.ts` 默认 public settings 删除 `custom_menu_red_dot_enabled`。
- [x] `composables/useCustomMenuRedDot.ts`：
  - [x] storage key 改为 `custom-menu-seen:<userId>:<itemId>:<version>`。
  - [x] 拆分为 `useCustomMenuRedDotRegistry`（顶层生命周期，引用计数管理全局 storage listener）+ 纯函数 `isCustomMenuDotVisibleFor` / `dismissCustomMenuDotFor` + 单实例便利版 `useCustomMenuRedDot`。
  - [x] 单实例版新增 `itemId` computed 参数；`enabled` 语义变为 item 级 show_red_dot。
- [x] `components/layout/AppSidebar.vue`：换成 registry + `flagCustomMenuDotForItem(item)` 闭包生成 per-item `showDot` flag；`handleMenuItemClick` 中改为 dismiss 被点击的那一项。
- [x] `views/user/CustomPageView.vue`：composable 传入 `itemId`；`enabled` 改为读取当前 route 对应项的 `show_red_dot`。
- [x] `views/admin/SettingsView.vue`：
  - [x] 删除自定义菜单卡片顶部的全局红点开关块，替换为 version 只读展示条。
  - [x] 每个菜单项表单在 `docUrl` 与 `iconSvg` 之间插入一个"在该菜单项上展示红点提醒" Toggle 行，绑定 `item.show_red_dot`。
  - [x] `form.custom_menu_items` 类型加 `show_red_dot: boolean`；`addMenuItem` 默认 false；`normalizeMenuItems` 显式将 `show_red_dot` 归一为 boolean。
  - [x] 提交路径删除 `custom_menu_red_dot_enabled` 字段。
- [x] i18n `en.ts` / `zh.ts`：从 `admin.settings.customMenuSecurity` 删除 `redDotEnabled` / `redDotEnabledHint`（保留 `redDotCurrentVersion`）；在 `admin.settings.customMenu` 新增 `showRedDot` / `showRedDotHelp` 两条。
- [x] `npx vue-tsc --noEmit` 全绿。

## 3. Spec

- [x] 覆盖 `custom-menu-red-dot` capability 的 4 条 requirement：admin 每项开关、hash 不含 show_red_dot、public settings 只带 version、每项独立 dismiss。
- [x] `openspec validate refactor-custom-menu-red-dot-per-item --strict` 通过。

## 4. Rollout

- 首次上线时，因 hash payload 移除 `enabled`，`custom_menu_version` 会一次性变化 → 已 dismiss 老 version 的用户会看到一次红点（若 admin 已给某项开启 show_red_dot）。这是期望行为。
- 数据库无需迁移；`custom_menu_red_dot_enabled` 遗留 setting row 保留但被忽略（也可后续在 defaults 清理任务中删除，非本次强制项）。
