# Sub2API 本地完整跑通设计文档

## 问题定义

当前公网 `https://api.aaccx.pw` 仍然白屏，浏览器控制台继续出现：

- `GET https://api.aaccx.pw/assets/vendor-vue-DdvVI69T.js 403`
- `GET https://api.aaccx.pw/assets/vendor-i18n-DY-5nrdT.js 403`
- `GET https://api.aaccx.pw/assets/vendor-misc-DJoKcLuU.js 403`
- `Failed to fetch dynamically imported module: https://api.aaccx.pw/assets/HomeView-DV-G3zoc.js`

这个现象说明前面的 nginx 响应体替换没有解决根问题。继续在公网边缘层追加 rewrite 风险太高，必须先建立一个本地可重复跑通的 Sub2API 基准，再从基准产物切回公网。

## 目标

先在本机证明 Sub2API 整体流程独立可用：

1. 后端直连入口可用：`http://127.0.0.1:18080`
2. 前端页面可完整加载，无白屏、无 chunk load error。
3. 前端本地加载不依赖 Cloudflare、nginx、`api.aaccx.pw` 或静态资源路径替换。
4. Sub2API 使用本地 CLIProxyAPI 上游完成真实 API 请求。
5. 用户 Key、分组、账号池、用量记录和扣费链路可验证。
6. 重新构建后的前端产物不再生成 `/assets/vendor-*` 文件名，公网不再触发 Cloudflare 对 `vendor-*` 路径的 403。

## 当前事实

### 本机端口

| 组件 | 地址 | 状态 | 说明 |
|---|---|---|---|
| CLIProxyAPI | `127.0.0.1:8317` | 监听中 | 本地账号池上游，只能内网访问 |
| Sub2API | `127.0.0.1:18080` | 监听中 | 当前真实 Sub2API 服务入口 |
| nginx | `*:8080` | 监听中 | Cloudflare Tunnel 后面的公网边缘入口 |

### 本地直连验证

已验证：

- `http://127.0.0.1:18080/health` 返回 `{"status":"ok"}`。
- `http://127.0.0.1:18080/assets/vendor-vue-DdvVI69T.js` 返回 200。
- `http://127.0.0.1:18080/assets/HomeView-DV-G3zoc.js` 返回 200。
- 本地直连 HTML 仍包含 `vendor-*` 引用。
- `HomeView-DV-G3zoc.js` 仍包含 `./index-DUHFzDC1.js` import。

结论：`vendor-*` 是当前 Vite 构建产物的真实文件名。本地直连不会 403，因为没有 Cloudflare；公网会 403，因为 Cloudflare 在边缘层直接拦截 `/assets/vendor-*`，origin nginx 收不到请求。

## 架构边界

### 本地基准链路

```text
浏览器
  -> http://127.0.0.1:18080
  -> Sub2API 后端 + 嵌入式前端
  -> http://host.docker.internal:8317/v1
  -> CLIProxyAPI 本地账号池
```

本地基准不经过：

- Cloudflare Tunnel
- `api.aaccx.pw`
- nginx `8080`
- nginx `sub_filter`
- 浏览器里旧的公网 Service Worker / cache

### 公网链路

```text
浏览器
  -> https://api.aaccx.pw
  -> Cloudflare Tunnel
  -> nginx 127.0.0.1:8080
  -> Sub2API 127.0.0.1:18080
  -> CLIProxyAPI 127.0.0.1:8317
```

公网层只应该做反向代理，不应该承担构建产物修复职责。前端 chunk 名称必须在 Sub2API 构建阶段稳定产出，不能依赖 nginx 响应体改写。

## 根因判断

根因不是“没有域名”，也不是单纯浏览器缓存。

真正问题是：`frontend/vite.config.ts` 的 `manualChunks()` 明确返回了这些 chunk 名称：

- `vendor-vue`
- `vendor-ui`
- `vendor-chart`
- `vendor-i18n`
- `vendor-misc`

Vite 最终会生成 `/assets/vendor-*.js`。这些路径在 Cloudflare 上被 403。只在 nginx 上把 HTML 或部分 JS 改成 `pkg-*`，无法可靠覆盖所有运行时 import、preload、缓存和未来构建产物。

正确修复方向：修改 Vite chunk 命名，从源头不再生成 `vendor-*`。

## 本地跑通方案

### 方案选择

采用“源头修复 + 本地嵌入式构建验收”：

1. 修改 `frontend/vite.config.ts` 的 `manualChunks()` 返回值，把 `vendor-*` 改成不会被 Cloudflare 拦截的名称。
2. 推荐命名：
   - `vendor-vue` -> `pkg-vue`
   - `vendor-ui` -> `pkg-ui`
   - `vendor-chart` -> `pkg-chart`
   - `vendor-i18n` -> `pkg-i18n`
   - `vendor-misc` -> `pkg-misc`
3. 执行前端构建，产物输出到 `backend/internal/web/dist`。
4. 执行后端 embed 构建，确保新前端被嵌入 Go 二进制或容器镜像。
5. 本地启动 Sub2API 到 `127.0.0.1:18080`。
6. 用 HTTP 和浏览器两层验收。

不再继续扩大 nginx `sub_filter`，因为这属于边缘 workaround，不能保证每次构建、每个 chunk、每个浏览器缓存都一致。

## 本地运行拓扑

### 已有运行态路径

当前已有运行态文件：

- `deploy/.env.scheme-a.local`
- `deploy/.env.scheme-a.runtime`

敏感值不写入文档。

其中 `deploy/.env.scheme-a.local` 已配置：

- `BIND_HOST=127.0.0.1`
- `SERVER_PORT=18080`
- `RUN_MODE=standard`
- `SECURITY_URL_ALLOWLIST_ALLOW_INSECURE_HTTP=true`
- `SECURITY_URL_ALLOWLIST_ALLOW_PRIVATE_HOSTS=true`

这适合本地连接 `host.docker.internal:8317` 上游。

### 依赖

| 依赖 | 用途 | 验收方式 |
|---|---|---|
| PostgreSQL | Sub2API 主数据 | migrations 完成，服务启动 |
| Redis | 缓存和队列 | 服务健康 |
| CLIProxyAPI | 本地账号池上游 | 使用内部 Key 请求 `/v1/models` 返回 200 |
| Sub2API | API 网关和前端 | `/health`、前端、用户 Key 请求都通过 |
| 浏览器 | 验证真实 chunk 加载 | 无 console error，无 failed request |

## 验收矩阵

### HTTP 层

| 验收项 | 命令 | 期望 |
|---|---|---|
| Sub2API health | `curl -sS http://127.0.0.1:18080/health` | `{"status":"ok"}` |
| 根 HTML | `curl -sS http://127.0.0.1:18080/` | 不包含 `/assets/vendor-` |
| Vue chunk | `curl -I http://127.0.0.1:18080/assets/pkg-vue-*.js` | HTTP 200 |
| 旧 vendor 路径 | `curl -I http://127.0.0.1:18080/assets/vendor-vue-*.js` | 新构建后不应存在或不被 HTML 引用 |
| HomeView chunk | `curl -sS http://127.0.0.1:18080/assets/HomeView-*.js` | 不包含 `vendor-` |
| Sub2API models | 使用测试用户 Key 请求 `/v1/models` | HTTP 200 |
| Sub2API chat | 使用测试用户 Key 请求 `/v1/chat/completions` | HTTP 200，返回测试内容 |
| Sub2API responses | 使用测试用户 Key 请求 `/v1/responses` | HTTP 200，返回 message 输出 |
| 用量记录 | 管理端或数据库查询 | 新增 usage log |

### 浏览器层

| 验收项 | 期望 |
|---|---|
| 打开 `http://127.0.0.1:18080/` | 页面渲染，不白屏 |
| Network failed requests | 0 |
| Console error | 0 个 chunk load error |
| 请求 URL | 不出现 `/assets/vendor-*` |
| 首页到登录页 | 路由正常 |
| 登录测试用户 | 成功进入 dashboard |
| dashboard 请求 API | `/api/v1/*` 正常返回 |

## 实施计划

### Task 1：建立本地前端资源失败断言

**文件：**

- 新增：`scripts/verify-local-frontend-assets.mjs`

**步骤：**

- [ ] 写 Node 脚本，请求 `http://127.0.0.1:18080/`，解析 HTML 中的 `/assets/*.js` 和 `/assets/*.css`。
- [ ] 断言 HTML 不包含 `vendor-`。
- [ ] 请求所有静态资源，断言 HTTP 200。
- [ ] 请求所有 JS，断言内容不包含 `vendor-`。
- [ ] 当前构建应失败，因为 HTML 仍包含 `vendor-*`。

### Task 2：从源头修改 Vite chunk 命名

**文件：**

- 修改：`frontend/vite.config.ts`

**步骤：**

- [ ] 将 `manualChunks()` 返回值从 `vendor-*` 改为 `pkg-*`。
- [ ] 不改变分包策略，只改名称。
- [ ] 执行 `pnpm --dir frontend run build`。
- [ ] 确认 `backend/internal/web/dist/assets/` 下不再生成 `vendor-*`。
- [ ] 执行 Task 1 的脚本，预期通过。

### Task 3：构建嵌入式后端并本地启动

**文件：**

- 使用：`Dockerfile`
- 使用：`deploy/docker-compose.dev.yml`
- 使用：`deploy/.env.scheme-a.local`

**步骤：**

- [ ] 确认本机 Docker CLI 可用；若当前 shell 找不到 `docker`，先修正 PATH 或使用 Docker Desktop 自带路径。
- [ ] 使用本地源码构建 Sub2API 镜像，确保新前端被嵌入。
- [ ] 以 `deploy/.env.scheme-a.local` 启动到 `127.0.0.1:18080`。
- [ ] 验证 `/health`。
- [ ] 验证根 HTML 不包含 `vendor-*`。
- [ ] 验证浏览器打开 `http://127.0.0.1:18080/` 不白屏。

### Task 4：恢复方案 A 业务链路

**文件：**

- 使用：`deploy/.env.scheme-a.runtime`
- 可能修改：数据库内 Sub2API account/group/user/key 配置

**步骤：**

- [ ] 确认 CLIProxyAPI 监听 `127.0.0.1:8317`。
- [ ] 确认 Sub2API account 指向 `http://host.docker.internal:8317/v1`。
- [ ] 确认 account `credentials.pool_mode=true`。
- [ ] 使用测试用户 Key 请求 `/v1/models`。
- [ ] 使用测试用户 Key 请求 `/v1/chat/completions`。
- [ ] 使用测试用户 Key 请求 `/v1/responses`。
- [ ] 查询用量记录，确认请求被 Sub2API 记录。

### Task 5：公网回切前检查

**文件：**

- 检查：`/opt/homebrew/etc/nginx/servers/cliproxy.conf`

**步骤：**

- [ ] 去掉临时 `vendor-` -> `pkg-` 的 nginx 响应体 rewrite。
- [ ] 让 nginx 只做普通反向代理到 `127.0.0.1:18080`。
- [ ] 保留必要的 websocket/header 转发。
- [ ] 执行 `nginx -t`。
- [ ] 重启 nginx。
- [ ] 用公网 `https://api.aaccx.pw/` 验证 HTML 不包含 `vendor-*`。
- [ ] 用浏览器验证公网无白屏、无 `/assets/vendor-*` 请求。

## 推荐最终 nginx 责任边界

nginx 只负责：

- TLS/Tunnel 后的 HTTP 反代
- 传递真实 IP 和协议头
- websocket/stream 连接稳定性
- body size 和 timeout

nginx 不负责：

- 修改前端 JS 内容
- 改写 Vite chunk 文件名
- 清理浏览器缓存
- 绕过 Cloudflare 对某些路径的拦截

## 风险与回滚

| 风险 | 处理 |
|---|---|
| 修改 chunk 名后旧浏览器缓存仍请求旧 `vendor-*` | 新构建 hash 会变化；公网根 HTML 不再引用旧路径；必要时只对 HTML 加一次 `Clear-Site-Data` |
| Docker CLI 当前不可用 | 先修 PATH 或使用已有容器管理方式；不影响设计结论 |
| CLIProxyAPI 内部账号池返回 401/403 | 保持 Sub2API account `pool_mode=true`，避免整个聚合账号被禁用 |
| 前端构建失败 | 不切公网，先修本地构建 |
| API 通但前端不通 | 以浏览器验收为准，不再只用 `/v1/models` 判断完成 |

## 完成定义

只有同时满足以下条件，才认为 Sub2API 本地完整跑通：

- `http://127.0.0.1:18080/health` 通过。
- `http://127.0.0.1:18080/` HTML 不包含 `vendor-*`。
- 本地浏览器打开 `http://127.0.0.1:18080/` 不白屏。
- Network 中没有 `/assets/vendor-*` 请求。
- Console 中没有 chunk load error。
- 测试用户 Key 的 `/v1/models`、`/v1/chat/completions`、`/v1/responses` 都返回 200。
- 用量记录可查询到新增请求。
- 公网切回前，nginx 不再依赖 `sub_filter` 修改构建产物。
