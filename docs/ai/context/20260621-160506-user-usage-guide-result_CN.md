# 用户面板使用方法页面结果

## 结果

已在普通用户登录后的左侧导航栏新增「使用方法」入口，路由为 `/usage-guide`。

页面展示 8 个使用步骤和用户提供的 10 张截图：

1. 访问 `aaccx.pw/shop` 页面，点击图中的进入按钮。
2. 新用户注册，老用户登录。
3. 选择订阅的页面，选择合适的套餐。
4. 完成支付后，悠一会给你一个兑换码。
5. 兑换成功后，去 API Key 页面生成密钥。
6. 选择分组，并且可以设置高级功能。
7. 启动 cc-switch，粘贴 API Key 和请求端口。
8. 保存配置后，重启 Codex，即可使用。

第 4 步展示第 4、5 张截图，第 7 步展示第 8、9 张截图，第 8 步按要求把第 10 张截图放在步骤文字上方。

## 修改边界

- 只修改前端用户面板页面、普通用户侧栏、路由、i18n 文案、测试和静态截图资源。
- 管理员页面和管理员侧栏未添加「使用方法」入口。
- 后端、API、支付、订单、订阅、兑换码、API Key、计费和公网 nginx 配置均未修改。

## 关键文件

- `frontend/src/views/user/UsageGuideView.vue`
- `frontend/src/router/index.ts`
- `frontend/src/components/layout/AppSidebar.vue`
- `frontend/src/assets/usage-guide/*.png`
- `frontend/src/views/user/__tests__/UsageGuideView.spec.ts`
- `frontend/src/components/layout/__tests__/AppSidebar.spec.ts`

## 验证

- `pnpm vitest run src/views/user/__tests__/UsageGuideView.spec.ts src/components/layout/__tests__/AppSidebar.spec.ts`
- `pnpm vitest run src/views/user/__tests__/UsageGuideView.spec.ts src/components/layout/__tests__/AppSidebar.spec.ts src/router/__tests__/guards.spec.ts`
- `pnpm typecheck`
- `pnpm build`

以上命令均已通过。构建输出仍包含项目既有的 Vite chunk 警告、Browserslist 数据提示和 Node deprecation 警告，不影响构建结果。
