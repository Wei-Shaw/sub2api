# 用户面板使用方法页面设计与计划

## 背景

用户希望在登录后的普通用户左侧导航栏增加「使用方法」栏目，用于在 `aaccx.pw` 的 Sub2API 用户面板中展示 8 个使用步骤和 10 张截图。

## 修改边界

- 只修改前端用户面板展示、路由、导航和相关前端测试。
- 不修改后端接口、数据库、鉴权、支付、订单、订阅、兑换码、API Key 生成或计费逻辑。
- 不在管理员页面或管理员侧栏的「我的账户」区域增加入口。
- 页面不新增操作按钮，只展示步骤文字和图片。
- 图片作为前端静态资源随构建发布，不依赖外部图床。

## 设计

- 新增登录后路由 `/usage-guide`，路由 meta 使用：
  - 标题：`使用方法`
  - 描述：`查看从购买订阅到配置 cc-switch 的完整步骤`
  - `requiresAuth: true`
  - `requiresAdmin: false`
- 侧栏只在普通用户 `userNavItems` 中展示「使用方法」，位置放在「兑换」之后、「个人资料」之前。
- 管理员菜单和管理员个人区不添加该导航项。
- 页面组件 `frontend/src/views/user/UsageGuideView.vue` 使用现有 `AppLayout`。
- 页面按 8 个步骤纵向展示，每个步骤包含步骤编号、标题文字和截图区域。
- 图片对应关系：
  - 步骤 1：第 1 张图片。
  - 步骤 2：第 2 张图片。
  - 步骤 3：第 3 张图片。
  - 步骤 4：第 4、5 张图片。
  - 步骤 5：第 6 张图片。
  - 步骤 6：第 7 张图片。
  - 步骤 7：第 8、9 张图片。
  - 步骤 8：第 10 张图片放在步骤文字上方。
- 样式延续黑白灰用户面板视觉，图片宽度自适应，不在移动端撑破布局。

## 实现计划

1. 新增用户指南页面测试，先确认当前缺少页面、步骤与图片映射。
2. 新增侧栏源码测试，先确认普通用户导航没有「使用方法」且管理员个人区不会出现。
3. 复制 10 张用户提供的 PNG 到 `frontend/src/assets/usage-guide/`，使用稳定文件名。
4. 新建 `UsageGuideView.vue`，用数组声明步骤和图片映射，模板只渲染文字与图片。
5. 更新 `frontend/src/router/index.ts` 增加 `/usage-guide` 登录路由。
6. 更新 `frontend/src/components/layout/AppSidebar.vue`，只在 `buildSelfNavItems(true)` 的普通用户菜单中插入「使用方法」。
7. 更新 `frontend/src/i18n/locales/zh.ts` 和 `frontend/src/i18n/locales/en.ts` 增加导航与页面文案。
8. 更新 `AGENTS.md` 记录本次用户面板使用方法页面约定。
9. 运行相关前端测试、类型检查和构建验证。

## 测试计划

- 先运行新增测试确认红灯。
- 实现后运行：
  - `pnpm vitest run src/views/user/__tests__/UsageGuideView.spec.ts src/components/layout/__tests__/AppSidebar.spec.ts src/router/__tests__/guards.spec.ts`
  - `pnpm typecheck`
  - `pnpm build`
  - `git diff --check`

## Tradeoff

- 不复用后台自定义 Markdown 页面：当前内容是固定产品引导，代码内静态页面更可控，避免增加后台配置和运行态依赖。
- 不增加管理员入口：用户明确要求管理员页面不需要添加，管理员仍可直接访问用户路由，但侧栏不提供入口。
- 不增加跳转按钮：需求只要求展示图片和文字，页面避免引导用户产生误操作。
