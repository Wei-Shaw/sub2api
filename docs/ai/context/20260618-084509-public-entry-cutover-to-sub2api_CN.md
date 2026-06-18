# 公网入口切换到 Sub2API 验收记录

## 背景

本次继续执行方案 A：公网只暴露 Sub2API，CLIProxyAPI 退为本机内网上游账号池，yui.web/shop 退为说明和跳转入口。

同时完成 yui.web 的 PR 流程：PR #38 已合并到远端 `main`，merge commit 为 `dad8f018efd8f6510649fd43e4c6970299dcfb13`。远端临时分支 `codex/sub2api-shop-entry` 已删除。

## 公网映射实际链路

调查确认当前公网入口不是直接映射 CLIProxyAPI 进程，而是：

```text
Cloudflare Tunnel
  -> 127.0.0.1:8080
  -> nginx server_name api.aaccx.pw
  -> upstream
```

切换前，nginx 的 `api.aaccx.pw` upstream 指向 `http://127.0.0.1:8317`，等价于把 CLIProxyAPI 暴露给公网。

切换后：

```text
公网 https://api.aaccx.pw
  -> Cloudflare Tunnel
  -> nginx 127.0.0.1:8080
  -> Sub2API 127.0.0.1:18080
  -> CLIProxyAPI 127.0.0.1:8317
  -> 本地账号池
```

CLIProxyAPI 仍监听 `127.0.0.1:8317`，不直接对公网开放。

## 本次运行态变更

- 修改 `/opt/homebrew/etc/nginx/servers/cliproxy.conf`：
  - 注释改为 Sub2API public gateway。
  - `proxy_pass` 从 `http://127.0.0.1:8317` 改为 `http://127.0.0.1:18080`。
- 执行 `nginx -t`，配置语法通过。
- 执行 `brew services restart nginx`，nginx 重启成功。
- 修改 `/Users/wujianxiang/CodeSpace/yui.web/.env`：
  - `SUB2API_PUBLIC_URL=https://api.aaccx.pw`
  - `SHOP_LEGACY_KEY_ISSUANCE_DISABLED=true`
- 重启 yui.web 本地 `node server.js`，让 `.env` 生效。

未在文档中记录完整 API Key、内部 token、HMAC secret 或密码。

## 验证结果

| 验证项 | 结果 |
| --- | --- |
| `http://127.0.0.1:8080/health` | HTTP 200，返回 Sub2API health |
| `https://api.aaccx.pw/health` | HTTP 200，返回 Sub2API health |
| 公网 `/v1/models` 使用 Sub2API 用户 Key | HTTP 200，返回模型列表 |
| 公网 `/v1/chat/completions` 使用 Sub2API 用户 Key | HTTP 200，返回 `pong` |
| 本机 8080 使用错误 Key | HTTP 401，错误码 `INVALID_API_KEY` |
| yui.web `/shop/` | 按钮链接渲染为 `https://api.aaccx.pw` |
| yui.web `/shop/guide/` | 说明页强调只使用 Sub2API 用户 Key |
| yui.web legacy 发 Key 接口 | HTTP 410，错误码 `SHOP_LEGACY_KEY_ISSUANCE_DISABLED` |
| 监听端口 | CLIProxyAPI 仅 `127.0.0.1:8317`，Sub2API 仅 `127.0.0.1:18080`，nginx 对外监听 `8080` |

## yui.web 本地同步说明

远端 `main` 已更新到 `dad8f018efd8f6510649fd43e4c6970299dcfb13`。

原 `/Users/wujianxiang/CodeSpace/yui.web` 工作区当前存在用户未提交改动、历史本地提交和已检出的本地分支，不能安全执行强制 reset 或删除本地分支。本次使用干净 worktree `/Users/wujianxiang/CodeSpace/yui.web/.worktrees/sub2api-shop-entry-clean` 拉取远端 main，并切到 `origin/main` detached 状态作为本地已同步视图。

## 后续注意事项

- `api.aaccx.pw` 现在是 Sub2API Base URL，朋友配置 Codex 时应使用 `https://api.aaccx.pw/v1`。
- 不要再把 CLIProxyAPI 内部 Key 发给朋友。
- 如果后续变更 Cloudflare Tunnel 或 nginx 8080 入口，必须确认 `api.aaccx.pw` upstream 仍指向 Sub2API，而不是 CLIProxyAPI。
- yui.web `.env` 是本地运行配置，含敏感项，不要提交。
