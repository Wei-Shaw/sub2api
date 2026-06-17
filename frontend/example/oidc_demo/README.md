# Sub2API OIDC — RP Demo

一个**零依赖、可实际运行**的 OIDC 接入示例（Relying Party / 信赖方）。

采用 BFF（Backend-for-Frontend）模式：浏览器只与本地 Node 服务交互，
`client_secret` 始终保存在服务端、不下发到前端。仅使用 Node 内置模块
（`http` / `crypto` / `fs`）+ 全局 `fetch`，无需 `npm install`。

## 实现的完整流程

1. **Discovery** — `GET {issuer}/.well-known/openid-configuration`
2. **Authorization (PKCE)** — 跳转到 `authorization_endpoint`，带 `state` / `nonce` / `code_challenge(S256)`
3. **Callback + Token 交换** — `POST {token_endpoint}`（`client_secret_basic` 认证）
4. **ID Token 验签** — RS256，通过 JWKS 按 `kid` 取公钥，并校验 `iss` / `aud` / `exp` / `nonce`
5. **UserInfo** — `GET {userinfo_endpoint}`
6. **Refresh（轮换）** — `POST {token_endpoint}` `grant_type=refresh_token`，持久化新 token、丢弃旧的

## 前置条件

- Node.js **>= 18**（需要全局 `fetch` 与 JWK 公钥支持；建议 20+）
- 在 Sub2API 管理后台的「OIDC 客户端」页面注册一个客户端，拿到 `client_id` / `client_secret`，
  并把回调地址 `http://localhost:3000/callback` 登记为允许的 redirect URI（需逐字节匹配）。

## 运行

```bash
cd frontend/example/oidc_demo
cp .env.example .env
# 编辑 .env，填入 SUB2API_ISSUER_URL / SUB2API_CLIENT_ID / SUB2API_CLIENT_SECRET
npm start
```

打开 http://localhost:3000 ，点击 **Login with Sub2API** 走完整流程。
登录后页面会展示：已验签的 ID Token claims、token 响应概要，并可点击按钮触发
**Refresh** 与实时 **UserInfo**。

## 配置项（`.env`）

| 变量 | 说明 |
|---|---|
| `SUB2API_ISSUER_URL` | Sub2API 实例 issuer 地址（结尾不带 `/`） |
| `SUB2API_CLIENT_ID` | 注册得到的客户端 ID |
| `SUB2API_CLIENT_SECRET` | 客户端密钥（仅服务端持有） |
| `SUB2API_REDIRECT_URI` | 回调地址，默认 `http://localhost:3000/callback` |
| `SUB2API_SCOPES` | 空格分隔的 scope；`openid` 必填，`offline_access` 启用 refresh |
| `PORT` | 本地端口，需与 redirect URI 中的端口一致 |

## 说明

> 仅用于演示。会话保存在内存（重启即失效），生产环境请改用持久化、安全的会话存储，
> 并完善错误处理、token 过期主动刷新、CSRF/Cookie 安全属性等。
