# Tasks

## Backend

### T1. 新增 setting keys
- [x] `internal/service/domain_constants.go`：新增
  - `SettingKeyCustomMenuRedDotEnabled = "custom_menu_red_dot_enabled"`
  - `SettingKeyCustomMenuVersionCache  = "custom_menu_version_cache"`
- [x] 在 `SystemSettings`（`settings_view.go`）struct 追加
  - `CustomMenuRedDotEnabled bool`
  - `CustomMenuVersionCache  string`
- [x] 在 defaults / seed 逻辑中，`CustomMenuRedDotEnabled` 默认 `false`。

### T2. Version 计算函数
- [x] 新增 `internal/service/custom_menu_version.go` 实现
  ```go
  func ComputeCustomMenuVersion(itemsJSON string, enabled bool) string
  ```
  - 步骤：Unmarshal → 按 `sort_order` 稳定排序 → map[key]value 按字母序 emit → 拼接 `{"enabled":<bool>,"items":<...>}` → SHA-256 → hex 前 12 字节。
  - 单元测试覆盖：字段重排相同、无序 items 相同、`enabled` 变化不同、items 内容变化不同、空 items 稳定值。
- [x] 单元测试：`internal/service/custom_menu_version_test.go`。

### T3. 保存路径接入 version 计算
- [x] `internal/handler/admin/setting_handler.go` `UpdateSettings`：
  - 新增读取 `req.CustomMenuRedDotEnabled *bool`。
  - 在 items JSON 或 enabled 任一变化后，重算 `newVersion`；无变化时保留旧 `CustomMenuVersionCache`。
  - 写回 `newSettings.CustomMenuRedDotEnabled` / `newSettings.CustomMenuVersionCache`。
  - `checkCustomMenuChanged` audit：追加 `custom_menu_red_dot_enabled` diff 键。
- [x] `admin` DTO `UpdateSystemSettingsRequest` 新增 `CustomMenuRedDotEnabled *bool`。
- [x] Admin `SystemSettings` 响应 DTO 新增只读字段 `CustomMenuRedDotEnabled bool`, `CustomMenuVersion string`。

### T4. Public settings 暴露新字段
- [x] `internal/handler/dto/settings.go`：`SystemSettings`（public 版本）追加
  - `CustomMenuRedDotEnabled bool   \`json:"custom_menu_red_dot_enabled"\``
  - `CustomMenuVersion       string \`json:"custom_menu_version"\``
- [x] `internal/handler/setting_handler.go`：装配 public 响应时填入。
- [x] `internal/server/api_contract_test.go`：更新期望 payload 快照。

### T5. Backend 联调
- [x] `go test ./internal/service/... ./internal/handler/...` 通过。
- [x] Manual：admin 保存 → GET public 看到 `custom_menu_version` 变化。

## Frontend

### T6. 类型 & store
- [x] 更新 `stores/appStore` 中 `cachedPublicSettings` 类型：追加两字段。
- [x] 更新 `views/admin/adminSettingsStore` 或等价 store 里 `SystemSettings` 类型（若存在）。

### T7. Composable
- [x] 新增 `frontend/src/composables/useCustomMenuRedDot.ts`：
  - 复用 `useRechargePromoDot` 的模式：`sharedDismissTick`、`storage` 监听、`localStorage` 读写。
  - 输入：`{ userId: ComputedRef<number|null>, enabled: ComputedRef<boolean>, version: ComputedRef<string|undefined> }`。
  - 输出：`{ shouldShow, dismiss }`。
  - key 前缀 `custom-menu-seen:`。
- [x] 单元测试 `__tests__/useCustomMenuRedDot.spec.ts`：
  - enabled=false 不显示。
  - version 未定义不显示。
  - 首次 shouldShow=true；dismiss 后 false。
  - version 变化后再次 true。
  - `storage` 事件跨 tab 同步。

### T8. AppSidebar 集成
- [x] `frontend/src/components/layout/AppSidebar.vue`：
  - 引入 composable，把 `shouldShow` 作为 `redDot` 传入自定义菜单 nav item 的 template 位置。
  - 在 `customMenuToNavItem` 或渲染层新增红点 slot / conditional `<span class="red-dot">`（对齐充值 tab 样式）。
  - `handleMenuItemClick` 内识别到 path 属于自定义菜单（`path.startsWith('/custom/')` 或来自 `customMenuItemsForUser`）时调用 `dismiss()`。
- [x] 单元测试补充（`AppSidebar.spec.ts`）：mock enabled + version → 断言 nav item 上有红点元素；模拟点击 → 断言 dismiss 后红点消失。

### T9. CustomPageView 集成
- [x] `frontend/src/views/user/CustomPageView.vue`：`onMounted` / `watch(route.params.id)` 触发 `dismiss()`。
- [x] 单元测试：mount CustomPageView → 断言 localStorage key 被写入。

### T10. Admin Settings 面板
- [x] `frontend/src/views/admin/SettingsView.vue`（自定义菜单区块，line ~5521）：
  - 在 items table 之前新增一个 subsection：Toggle + description + `当前版本: <hash>`。
  - Toggle 双向绑定到 `form.custom_menu_red_dot_enabled`。
  - 提交时把该字段带上；hash 值只读展示（从 `props.settings.custom_menu_version` 读）。
- [x] 单元测试：`SettingsView.spec.ts` 补充 assert 该 toggle 值双向绑定 + payload 里带出。

### T11. i18n
- [x] `zh-CN.ts` / `en.ts` 新增：
  - `admin.settings.customMenu.redDot.title`
  - `admin.settings.customMenu.redDot.description`
  - `admin.settings.customMenu.redDot.currentVersion`
  - `admin.settings.customMenu.redDot.copyVersion`
  - `nav.customMenu.newBadgeAria`（红点的 aria-label，如 "新内容 / New"）

### T12. Frontend 联调
- [x] `pnpm test` 全通过。
- [x] Manual smoke：
  1. Admin 打开开关 → 保存 → 用户看到红点。
  2. 用户点击任意自定义菜单项 → 红点消失。
  3. 打开一个新 tab 未 dismiss 的用户界面 → dismiss 后红点也消失（storage 事件）。
  4. Admin 修改任一 item label → 红点重新出现。
  5. Admin no-op 保存（同样内容） → 红点保持已 dismiss，不刷新。
  6. Admin 关闭开关 → 红点立即消失（无论 dismiss 与否）。
  7. 移动端 drawer 也能看到并 dismiss 红点。
  8. 直接输入 URL `/custom/:id` → 红点也 dismiss。

## Documentation

- [x] （可选）在 `openspec/specs/console-navigation/spec.md` 里追加对自定义菜单红点的一条 requirement 引用（本次通过 spec delta 完成，archive 时 openspec 会自动 merge）。
