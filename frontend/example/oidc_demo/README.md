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
  并把回调地址 `http://localhost:4000/callback` 登记为允许的 redirect URI（需逐字节匹配）。

## 运行

```bash
cd frontend/example/oidc_demo
# 编辑 .env，填入 SUB2API_ISSUER_URL / SUB2API_CLIENT_ID / SUB2API_CLIENT_SECRET
npm start
```

打开 http://localhost:4000 ，点击 **Login with Sub2API** 走完整流程。
登录后页面会展示：已验签的 ID Token claims、token 响应概要，并可点击按钮触发
**Refresh** 与实时 **UserInfo**。

## 配置项（`.env`）

| 变量 | 说明 |
|---|---|
| `SUB2API_ISSUER_URL` | Sub2API 实例 issuer 地址（结尾不带 `/`） |
| `SUB2API_CLIENT_ID` | 注册得到的客户端 ID |
| `SUB2API_CLIENT_SECRET` | 客户端密钥（仅服务端持有） |
| `SUB2API_REDIRECT_URI` | 回调地址，默认 `http://localhost:4000/callback` |
| `SUB2API_SCOPES` | 空格分隔的 scope；`openid` 必填，`offline_access` 启用 refresh |
| `PORT` | 本地端口，需与 redirect URI 中的端口一致 |

## 本地 https（方案 B：让 issuer 走 https://localhost）

OIDC 的 **issuer 必须是 https**（前后端都强制校验），但本地 sub2api 通常是
http。方案 B 用 [mkcert](https://github.com/FiloSottile/mkcert) 签发受信任的
localhost 证书，再用 [Caddy](https://caddyserver.com/) 在 `:8443` 套一层 https
反代，从而得到合法的 `https://localhost:8443` 作为 issuer。

> **重要**：OIDC 登录流程会同时用到后端 API 和**前端页面**（`/login`、
> `/oauth/consent`）。所以 issuer 背后必须同时能访问到「后端 + 前端」：
> - 开发模式（前端跑 vite dev，默认 `:3000`；后端 `:8080`）：本目录 `Caddyfile`
>   已按路径分流——`/oidc`、`/api`、`/v1`、`/setup`、`/.well-known` → 后端，
>   其余 → 前端。**记得 sub2api 前端 dev server 也要起着**，否则 `/login` 会 404。
> - 内嵌前端的后端（`go build -tags embed`，前端打包进二进制）：前端页面也由
>   `:8080` 提供，可把 `Caddyfile` 的分流删掉，直接反代 `127.0.0.1:8080`。

> 只有 **issuer** 需要 https；demo 自身的 redirect_uri 仍可用
> `http://localhost:4000/callback`（后端对 `http://localhost` 回调放开）。
>
> 注意区分端口：sub2api 前端 dev 是 `:3000`，本 demo（独立 RP）是 `.env` 里的
> `PORT=4000`，两者不要混淆；demo 不经过 Caddy。

### 1. 安装 mkcert 并签发证书

```bash
sudo apt install -y libnss3-tools
curl -JLO "https://dl.filippo.io/mkcert/latest?for=linux/amd64"
chmod +x mkcert-v*-linux-amd64 && sudo mv mkcert-v*-linux-amd64 /usr/local/bin/mkcert

mkcert -install
cd frontend/example/oidc_demo
mkcert localhost 127.0.0.1 ::1   # 生成 localhost+2.pem / localhost+2-key.pem
```

### 2. 启动 Caddy 反代（本目录已提供 `Caddyfile` 与脚本）

```bash
# 安装 caddy 见 https://caddyserver.com/docs/install
./start-caddy.sh        # 后台启动，不占用终端
# 验证 discovery 走通（返回 JSON 即成功）：
curl https://localhost:8443/.well-known/openid-configuration
# 用完停止：
./stop-caddy.sh
```

`start-caddy.sh` 会校验 caddy 已安装、证书已签发，再用 `caddy start` 在**后台**启动
（由 Caddy 自带 admin API 管理）。`Caddyfile` 默认监听 `:8443` 并反代到后端
`127.0.0.1:8080`，按实际端口自行调整。

### 3. 两处 issuer 保持一致

- 管理后台「OIDC 设置」→ issuer URL：`https://localhost:8443`
- 本目录 `.env`：`SUB2API_ISSUER_URL=https://localhost:8443`（结尾不带 `/`）

### 4. 用信任私有 CA 的方式启动 demo

Node 默认不认 mkcert 的私有根证书，直接 `npm start` 访问 issuer 会报
`fetch failed` / `self-signed certificate`。用本目录的脚本启动（已自动注入根证书）：

```bash
./start-https.sh
```

> 等价于：`NODE_EXTRA_CA_CERTS="$(mkcert -CAROOT)/rootCA.pem" npm start`

## 说明

> 仅用于演示。会话保存在内存（重启即失效），生产环境请改用持久化、安全的会话存储，
> 并完善错误处理、token 过期主动刷新、CSRF/Cookie 安全属性等。
