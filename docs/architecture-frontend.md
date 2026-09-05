# Frontend 架构文档

**生成时间:** 2026-04-28  
**扫描级别:** Quick Scan  
**范围:** `frontend/`

## 概览

前端是 Vue 3 单页应用，提供用户控制台、管理员后台、支付购买、认证回调、渠道监控、运维监控和设置页面。构建产物输出到 `backend/internal/web/dist`，生产发布时由 Go 后端内嵌服务。Quick Scan 基于 `package.json`、`pnpm-lock.yaml`、`vite.config.ts`、`router/index.ts`、`api/client.ts`、目录清单和现有 README 生成。

## 技术栈

| 类别 | 技术 | 版本/来源 | 用途 |
| --- | --- | --- | --- |
| 框架 | Vue | `3.5.26` in `pnpm-lock.yaml` | SPA 组件模型 |
| 构建 | Vite | `5.4.21` | dev server、构建和代理 |
| 语言 | TypeScript | `5.6.3` | 类型检查 |
| 状态 | Pinia | `2.3.1` | 全局状态 |
| 路由 | Vue Router | `4.6.4` | 页面导航、鉴权守卫 |
| HTTP | Axios | `1.15.0` | API client |
| 样式 | TailwindCSS | `3.4.19` | Utility CSS、暗色模式 |
| i18n | vue-i18n | `9.14.5` | 中英文文案和 CSP 兼容 JIT |
| 测试 | Vitest + jsdom + Vue Test Utils | `2.1.9` | 单元/组件测试 |

## 入口与构建

- `frontend/src/main.ts`：SPA 启动入口。
- `frontend/src/App.vue`：根组件。
- `frontend/src/router/index.ts`：完整路由表、认证守卫、backend mode/simple mode 限制、动态 import 错误恢复。
- `frontend/src/api/client.ts`：Axios 实例、token 注入、Accept-Language、GET timezone、统一响应解包和 refresh token。
- `frontend/vite.config.ts`：`@` alias、i18n runtime alias、dev public settings 注入、dev proxy、构建输出路径。

## 路由结构

| 分组 | 典型路径 | 页面目录 | 说明 |
| --- | --- | --- | --- |
| Setup | `/setup` | `views/setup` | 首次安装向导 |
| Public/Auth | `/home`, `/login`, `/register`, `/auth/*`, `/forgot-password`, `/reset-password`, `/key-usage` | `views/auth`, root views | 登录、注册、OAuth 回调、公开 key usage |
| User | `/dashboard`, `/keys`, `/usage`, `/redeem`, `/profile`, `/subscriptions`, `/purchase`, `/orders`, `/monitor` | `views/user` | 用户控制台、API key、用量、订阅、支付、渠道状态 |
| Admin | `/admin/dashboard`, `/admin/users`, `/admin/groups`, `/admin/accounts`, `/admin/settings`, `/admin/ops`, `/admin/channels/*` | `views/admin` | 管理后台 |
| Payment Admin | `/admin/orders`, `/admin/orders/dashboard`, `/admin/orders/plans` | `views/admin/orders` | 支付订单、支付 dashboard、套餐 |
| Custom | `/custom/:id` | `views/user/CustomPageView.vue` | 后台配置的自定义菜单页面 |

路由 meta 使用 `requiresAuth`、`requiresAdmin`、`requiresPayment`、`titleKey`、`descriptionKey` 控制鉴权、支付开关和标题。

## 状态管理

Pinia store 位于 `frontend/src/stores`：

- `auth.ts`：用户身份、token、本地恢复、simple mode、pending auth。
- `app.ts`：站点公共配置、toast/loading、backend mode、站点名称、公开菜单。
- `adminSettings.ts`：管理员设置状态和自定义菜单。
- `payment.ts`：支付相关状态。
- `subscriptions.ts`：用户订阅状态。
- `announcements.ts`：公告状态。
- `onboarding.ts`：引导流程。

## API 层

API 模块位于 `frontend/src/api`，按领域切分。公共 client 规则：

- base URL 默认 `/api/v1`，可通过 `VITE_API_BASE_URL` 覆盖。
- 请求自动附加 `Authorization: Bearer <token>`。
- 请求自动附加 `Accept-Language`。
- GET 请求自动追加浏览器 timezone。
- 成功响应自动从 `{ code, data }` 解包到 `response.data`。
- 401 时基于 refresh token 排队刷新，失败后清理本地身份并跳转登录。

管理端 API 位于 `frontend/src/api/admin`，覆盖 accounts、users、groups、settings、ops、payment、channels、channel monitor、backup、data management 等领域。

## 组件组织

| 目录 | 文件数 | 说明 |
| --- | ---: | --- |
| `components/common` | 39 | 通用控件、表格、弹窗、表单、分页、toast、状态徽章 |
| `components/layout` | 7 | App shell、侧边栏、头部、认证布局、表格页面布局 |
| `components/account` | 31 | 账号容量、OAuth、配额、批量编辑、账号测试 |
| `components/admin` | 51 | 管理端领域组件，含 account/channel/group/monitor/payment/usage/user |
| `components/payment` | 19 | 订单、支付方式、二维码、Stripe、套餐卡片 |
| `components/user` | 30 | 用户 dashboard、profile、monitor 等 |
| `components/auth` | 8 | OAuth、TOTP、pending OAuth 创建账号 |
| `components/charts` | 7 | 用量趋势和分布图表 |
| `components/keys` | 4 | API key 使用与 endpoint popover |

## 样式与 i18n

- 样式以 Tailwind utility class 为主，暗色模式依赖根元素 `dark` class。
- 全局 CSS 在 `frontend/src/style.css`，引导样式在 `frontend/src/styles/onboarding.css`。
- i18n 文件在 `frontend/src/i18n`，语言包为 `locales/en.ts` 和 `locales/zh.ts`。
- `vite.config.ts` 将 `vue-i18n` alias 到 runtime 版本，并启用 `__INTLIFY_JIT_COMPILATION__` 适配 CSP。

## 测试策略

- `pnpm run typecheck`：TypeScript/Vue 类型检查。
- `pnpm run test:run`：Vitest。
- `pnpm run test:coverage`：覆盖率。
- 根 Makefile 的 `make test-frontend` 会运行 lint check、typecheck 和关键 Vitest 子集。
- 测试 setup 位于 `frontend/src/__tests__/setup.ts`，已集中 mock 浏览器 API。

## 修改注意事项

- 通用 UI 优先复用 `components/common`，布局复用 `components/layout`。
- 新路由必须在 `router/index.ts` 加 meta，并考虑 backend mode、simple mode、requiresPayment。
- 新 API 调用应通过 `apiClient`，不要在组件中绕过 token、语言、timezone 和响应解包逻辑。
- 新文案应进入 i18n 字典，使用 key 引用。
- 修改 public settings 相关逻辑时要兼容 Vite dev 注入和后端生产注入两条路径。
