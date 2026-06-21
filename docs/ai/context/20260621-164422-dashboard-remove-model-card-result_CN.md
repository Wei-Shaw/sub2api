# 仪表盘移除模型拆分卡片结果

## 改动

- 已从用户仪表盘图表区域移除「模型分布」卡片，也就是截图中的按模型拆分卡片。
- 保留时间范围、刷新、粒度选择和「Token 使用趋势」卡片。
- 删除 `UserDashboardCharts.vue` 中仅服务模型卡片的 Doughnut 图表、Chart.js 注册、模型表格格式化和灰阶调色板依赖。
- 新增 `UserDashboardCharts` 组件测试，覆盖「模型分布」和模型名不再渲染，同时确认筛选区和 Token 趋势仍存在。

## 未改动

- 未修改后端、API、路由、计费、订阅、兑换码和公网配置。
- 未删除 `DashboardView.vue` 中模型统计数据加载逻辑，避免扩大本次 UI 删除的影响面。

## 验证

- `pnpm vitest run src/components/user/dashboard/__tests__/UserDashboardCharts.spec.ts`
  - 先在旧实现下失败，失败原因为页面仍包含「模型分布」。
  - 修改后通过：1 个测试通过。
- `pnpm build`
  - 通过，包含 `vue-tsc -b` 和 Vite production build。
  - 输出仅包含项目既有的 Browserslist、Vite 动态导入和 chunk size 提示。
