# Sub2API 本地完整跑通记录：源头 pkg chunk 修复

## 时间

2026-06-18 09:36 JST

## 本次目标

按 `20260618-091756-sub2api-local-runthrough-design_CN.md` 的要求，先在本机完整验证 Sub2API 基准链路：

```text
浏览器 / curl
  -> http://127.0.0.1:18080
  -> Sub2API Docker 容器
  -> http://host.docker.internal:8317/v1
  -> CLIProxyAPI 本地账号池
```

本次不继续扩大 nginx `sub_filter` workaround，改为从 Vite 构建源头消除 `/assets/vendor-*`。

## 代码修改

- `frontend/vite.config.ts`：`manualChunks()` 返回值从 `vendor-*` 改为 `pkg-*`；分包策略不变。
- `scripts/verify-local-frontend-assets.mjs`：新增本地资产断言脚本，递归检查 HTML 入口资源和 JS import，断言不包含 `vendor-`。
- `Dockerfile` / `deploy/Dockerfile`：前端构建阶段复制 `docs/legal/` 到 `/app/docs/legal/`。
- `.dockerignore`：只放开 Docker 构建需要的 `docs/legal/*.md`。
- `.gitignore`：显式允许跟踪 `scripts/verify-local-frontend-assets.mjs`。

## 发现的问题与根因

### 当前运行实例仍是旧产物

新增断言脚本后，先对旧的 `http://127.0.0.1:18080` 跑红灯：

```text
根 HTML 仍包含 vendor-
```

说明测试能捕获当前白屏根因，不是空断言。

### Docker CLI PATH 问题

当前 shell 找不到 `docker`，但本机存在：

```text
/Applications/Docker.app/Contents/Resources/bin/docker
```

后续构建使用：

```bash
PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH" docker ...
```

### Dockerfile 缺少 docs/legal

首次 Docker build 失败：

```text
Could not resolve "../../../../docs/legal/admin-compliance.zh.md?raw"
```

根因是前端代码 raw import `docs/legal/admin-compliance.*.md`，但 Dockerfile 只复制了 `frontend/`。本地 `pnpm --dir frontend run build` 从仓库根目录能访问 `docs/legal`，容器内不能。

修复后又发现 `.dockerignore` 忽略了整个 `docs/`，因此同步放开 `docs/legal/*.md`。

## 构建与启动

已执行：

```bash
pnpm --dir frontend install --frozen-lockfile
pnpm --dir frontend run build
PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH" docker build -t weishaw/sub2api:latest .
PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH" docker compose --env-file .env.scheme-a.local -f docker-compose.local.yml up -d --no-deps --force-recreate sub2api
```

结果：

- 前端构建通过。
- Docker 镜像构建通过。
- 仅重建 `sub2api` 应用容器；PostgreSQL / Redis 容器未重建、未清数据。
- `sub2api` 容器健康。

## HTTP 资产验证

已执行：

```bash
node scripts/verify-local-frontend-assets.mjs
```

结果：

```json
{
  "baseUrl": "http://127.0.0.1:18080",
  "htmlAssets": 6,
  "checkedAssets": 125,
  "vendorReferences": 0
}
```

根 HTML 当前入口资源：

```text
/assets/index-HOAcWbNE.js
/assets/pkg-vue-DdvVI69T.js
/assets/pkg-i18n-ECmPCSvH.js
/assets/pkg-misc-DRGW1HPS.js
/assets/pkg-misc-DB0Q8XAf.css
/assets/index-zS8LF1jO.css
```

`backend/internal/web/dist` 中没有 `vendor-*` 文件名，也没有 `vendor-` 内容引用。

注意：直连旧 `/assets/vendor-vue-DdvVI69T.js` 时，后端会按 SPA fallback 返回 HTML 200；判定标准不是旧路径返回码，而是新 HTML / JS 链路不再引用旧路径。

## 浏览器验证

使用 Codex In-app Browser 打开：

```text
http://127.0.0.1:18080/
```

结果：

- 页面自动路由到 `http://127.0.0.1:18080/home`。
- 标题：`Home - Sub2API`。
- 页面正文可见，非白屏。
- `#app` 有实际子节点。
- Console warn/error：0。
- DOM 引用中 `/assets/vendor-*`：0。
- 浏览器资源清单：
  - script：10
  - stylesheet：4
  - image：1
  - `/assets/vendor-*`：0
  - `/assets/pkg-*` 包含：
    - `/assets/pkg-vue-DdvVI69T.js`
    - `/assets/pkg-i18n-ECmPCSvH.js`
    - `/assets/pkg-misc-DRGW1HPS.js`
    - `/assets/pkg-misc-DB0Q8XAf.css`

## 方案 A 业务链路验证

DB 脱敏状态：

- 活跃测试 Key：`sk-39814...49c4`
- Account：`cliproxy-local-openai`
- `base_url`：`http://host.docker.internal:8317/v1`
- `credentials.pool_mode`：`true`
- `pool_mode_retry_count`：`3`
- `pool_mode_retry_status_codes`：`[401, 403, 429]`

已使用测试 Key 请求本地 Sub2API：

- `/v1/models`：HTTP 200，返回 10 个模型。
- 选用模型：`gpt-5.5`。
- `/v1/chat/completions`：HTTP 200，返回 `local-ok`。
- `/v1/responses`：HTTP 200，返回 `local-responses-ok`。
- `usage_logs`：从 6 增加到 8，新增 2 条。
- 最近新增记录显示：
  - `/v1/chat/completions` 入站，上游 `/v1/responses`
  - `/v1/responses` 入站，上游 `/v1/responses`

未在文档中记录完整 API Key 或敏感 token。

## 当前判断

本地完整链路已跑通：

- `http://127.0.0.1:18080/health` 通过。
- 本地嵌入式前端不再生成或引用 `/assets/vendor-*`。
- 浏览器打开本地页面不白屏，无 chunk load error。
- 方案 A 的 CLIProxyAPI 上游账号池可用。
- 测试 Key 的 models、chat、responses 都通过。
- 用量记录和扣费链路有新增记录。

下一步如果要回切公网，应先把 nginx 从临时 `sub_filter` workaround 收敛为普通反代，再验证 `https://api.aaccx.pw/` 是否直接拿到新 `pkg-*` 构建产物。
