## Why

在 `add-custom-menu-red-dot` 归档后，实际使用中我们发现"全局单开关 + 全局单 dismiss"的粒度太粗：admin 只能一键让所有自定义菜单项一起亮红点，用户点击任一项后所有项的红点都会消失。这既不符合"红点是提醒某项内容"的直觉，也让 admin 无法只针对个别新增/更新的项做提醒。

本次改动把红点粒度从"全局"改为"每项独立"：

- **admin**：删除 `custom_menu_red_dot_enabled` 全局开关，改为每个 `CustomMenuItem` 自带一个 `show_red_dot` boolean。
- **前端**：dismiss key 从 `custom-menu-seen:<userId>:<version>` 变成 `custom-menu-seen:<userId>:<itemId>:<version>`，每项独立记忆。
- **后端 hash**：`ComputeCustomMenuVersion` 只哈希展示字段，不再哈希 enabled/show_red_dot——admin 单纯切换某项开关不会重置其他用户对其它项的 dismiss 状态；只有真正修改展示字段（label/url/icon/顺序）才会 mint 新 version。

## What Changes

- **BREAKING (backend contract)**：删除 `custom_menu_red_dot_enabled` 字段（`SystemSettings` / `PublicSettings` / admin update request）。任何前端旧版本都不会因缺失此字段而崩溃（本仓库 monorepo 前后端一体上线）。
- **BREAKING (backend function signature)**：`ComputeCustomMenuVersion(itemsJSON, enabled)` → `ComputeCustomMenuVersion(itemsJSON)`；hash payload 由 `{"enabled": ..., "items": [...]}` 改为 `{"items": [...]}`。首次上线会一次性 mint 新 version（用户会看到一次红点），可接受。
- **NEW (backend contract)**：`CustomMenuItem` 新增 `show_red_dot bool json:"show_red_dot,omitempty"`，默认 false；参与序列化但**不参与** hash 计算。
- **BREAKING (frontend contract)**：`useCustomMenuRedDot` composable 新增 `itemId` 必填参数；同时导出 `useCustomMenuRedDotRegistry` + `isCustomMenuDotVisibleFor` / `dismissCustomMenuDotFor` 供 v-for 场景使用。
- **admin UI**：删除自定义菜单卡片顶部的全局开关行，改为每个 item 表单里插入一个 "在该菜单项上展示红点提醒" Toggle；version 展示保留为参考信息条。
- **spec 变更**：`custom-menu-red-dot` capability 全量重写关键 requirement，反映新的字段与粒度。

## Impact

- Affected specs: `custom-menu-red-dot`（覆盖 5 条 requirement 中的 4 条）。
- Affected code:
  - Backend: `internal/handler/dto/settings.go`, `internal/service/custom_menu_version.go`, `internal/service/setting_service.go`, `internal/service/settings_view.go`, `internal/service/domain_constants.go`, `internal/handler/setting_handler.go`, `internal/handler/admin/setting_handler.go`, 相关单测。
  - Frontend: `composables/useCustomMenuRedDot.ts`, `components/layout/AppSidebar.vue`, `views/user/CustomPageView.vue`, `views/admin/SettingsView.vue`, `types/index.ts`, `api/admin/settings.ts`, `stores/app.ts`, `i18n/locales/{en,zh}.ts`。
- Migration: 数据库层无需迁移——`custom_menu_red_dot_enabled` 遗留 setting row 会被后续 defaults 清理策略忽略；旧 items JSON 里没有 `show_red_dot` 字段的项默认按 false 处理。
