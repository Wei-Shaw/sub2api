# Frontend 开发指南

**生成时间:** 2026-04-28  
**范围:** `frontend/`

## 前置条件

- Node.js `20`，CI 使用该版本。
- pnpm `9`，CI 使用 `pnpm/action-setup@v4`。
- 后端本地服务默认可通过 `VITE_DEV_PROXY_TARGET` 指向，缺省为 `http://localhost:8080`。

## 安装与运行

```bash
cd frontend
pnpm install --frozen-lockfile
pnpm run dev
```

Vite dev server 默认监听：

- host: `0.0.0.0`
- port: `VITE_DEV_PORT` 或 `3000`
- proxy: `/api`、`/v1`、`/setup` 到 `VITE_DEV_PROXY_TARGET`

## 构建

```bash
cd frontend
pnpm run build
```

构建会执行 `vue-tsc -b && vite build`，产物输出到：

```text
backend/internal/web/dist
```

生产后端 release 会使用该目录做 embed 构建。

## 测试与质量检查

```bash
cd frontend
pnpm run lint:check
pnpm run typecheck
pnpm run test:run
pnpm run test:coverage
```

根目录 CI 风格入口：

```bash
make test-frontend
```

当前根 Makefile 的关键 Vitest 子集包括认证回调、支付、profile、admin settings 等高风险路径。

## API 开发规则

- 所有业务 API 请求通过 `frontend/src/api/client.ts` 的 `apiClient`。
- 不要绕过响应解包、token refresh、Accept-Language、timezone 注入。
- API 模块按领域放在 `frontend/src/api` 或 `frontend/src/api/admin`。
- 类型优先放在 `frontend/src/types` 或模块内清晰导出，类型导入使用 `import type`。

## 路由开发规则

新增页面时在 `frontend/src/router/index.ts` 加 route record，并检查：

- `requiresAuth`
- `requiresAdmin`
- `requiresPayment`
- `titleKey` / `descriptionKey`
- backend mode 公开路径白名单
- simple mode 限制
- OAuth pending flow 是否可访问

## UI 开发规则

- 通用控件优先复用 `components/common`。
- 后台表格页优先复用 `components/layout/TablePageLayout`、`DataTable`、`Pagination`、通用 filter/dialog 模式。
- 领域组件放到 `components/account`、`components/admin/*`、`components/payment`、`components/user/*` 等对应目录。
- 文案进入 `frontend/src/i18n/locales/en.ts` 和 `zh.ts`，组件使用 key。
- 暗色模式要配套检查 `dark:` class。

## Public Settings 注入

开发模式下，`vite.config.ts` 的 `injectPublicSettings` 会请求后端 `/api/v1/settings/public` 并注入 `window.__APP_CONFIG__`，生产模式由后端 `internal/web` 注入。改动站点公开配置时，两条路径都要兼容。
