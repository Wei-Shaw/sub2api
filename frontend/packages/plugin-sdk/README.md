# @sub2api/plugin-sdk

通用 UI 组件 SDK, 供 sub2api host frontend 与 plugin 共享.

## 公开组件

- `Icon` — lucide 图标包装
- `DataTable` — 数据表格 (含 server-side / client-side 排序)
- `BaseDialog` — 模态对话框基础组件
- `ConfirmDialog` — 确认对话框 (基于 BaseDialog)
- `Select` — 自定义下拉
- `Pagination` — 分页 (含 page size 持久化)
- `EmptyState` — 空状态展示
- `Toggle` — 开关
- `PlatformIcon` — 平台图标映射

## 公开类型

- `Column` — DataTable 列定义

## 公开工具

- `tablePreferences` — 表格偏好持久化工具

## 形态

- 源码发布 (`*.vue` + `*.ts`), 不预编译
- host vite & plugin vite 各自 transform
- tailwind class & `<style scoped>` 走各自 PostCSS 管线
