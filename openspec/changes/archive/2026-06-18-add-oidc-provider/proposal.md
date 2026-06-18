## Why

sub2api 当前只能作为 OIDC **客户端**接入外部 IdP（见 [auth_oidc_oauth.go](/home/jiantaoli/sub2api/backend/internal/handler/auth_oidc_oauth.go)），但越来越多的内部业务/合作方应用希望直接复用 sub2api 的账号体系做 SSO，并按用户身份调用 sub2api 之外的私有能力。把 sub2api 升级为标准 OIDC **提供者**（OP，OpenID Provider）后，第三方应用只要走 OIDC Authorization Code + PKCE 流程就能登录，无需再为 sub2api 写一套 cookie/token 自定义集成。当前 JWT 用 HS256 对称密钥（见 [auth_service.go](/home/jiantaoli/sub2api/backend/internal/service/auth_service.go) `GenerateTokenPair` 第 1197 行）、没有标准 Discovery / JWKS / Authorize / Token / UserInfo 端点，这条升级路径的所有基建都还不存在，必须新建。

## What Changes

- 新增能力：sub2api 自身成为 **standards-compliant OIDC Provider**，对外暴露下列 6 个端点：
  - `GET /.well-known/openid-configuration`：Discovery
  - `GET /.well-known/jwks.json`：公钥集（仅暴露公钥，永不暴露私钥）
  - `GET /oidc/authorize`：授权端点（仅支持 `response_type=code`）
  - `POST /oidc/token`：Token 端点（仅支持 `grant_type=authorization_code` 与 `refresh_token`）
  - `GET /oidc/userinfo`：UserInfo 端点（Bearer 鉴权）
  - `GET /oidc/consent` + `POST /oidc/consent`：Consent 页面与决策提交
- 新增 **OIDC Client 注册** —— Admin 后台手工录入 client_id / client_secret(hash) / redirect_uris / scopes / consent_required，落库到新增 ent schema `oidc_client`；不支持 RFC 7591 动态客户端注册
- 新增 **RSA 非对称签名密钥对**用于签发 OIDC ID Token —— 复用 `security_secrets` 表（见 [security_secret.go](/home/jiantaoli/sub2api/backend/ent/schema/security_secret.go)）；首次启动时自动生成 RSA-2048 keypair，私钥仅用于签名，公钥通过 JWKS 暴露；后续支持手动滚动新 kid
- 新增 **HttpOnly SSO Session Cookie** —— 用户在 sub2api 主站登录成功时（含密码登录、邮箱魔法链接、外部 OAuth 完成等所有路径）顺带 set 一个 HttpOnly+Secure+SameSite=Lax 的 sub2api SSO cookie，专供 `/oidc/authorize` 浏览器跳转识别登录态。前端现有 localStorage JWT 不变、不读不写
- 新增 **Authorization Code 短生命周期表** `oidc_authorization_code` —— 一次性、最长 10 分钟、bind PKCE code_challenge、bind redirect_uri 和 client_id；不复用 `pending_auth_sessions`（语义不符）
- 新增 **OIDC Refresh Token 表** `oidc_refresh_token` —— 与 sub2api 自身的 access token refresh 解耦，独立家族（family）轮换、可被吊销
- 新增 **OIDC Consent 决策表** `oidc_consent` —— 记录 (user_id, client_id, scopes_granted, granted_at)，按用户/客户端/scope 维度记忆同意结果
- 新增 **私有 scope** `sub2api:balance` 与 `sub2api:apikey`：
  - `sub2api:balance` → UserInfo 返回 `balance` claim（decimal 字符串）和 `total_recharged`
  - `sub2api:apikey` → UserInfo 返回 `apikey_count`（当前生效 API Key 数量），**不**返回明文 key
- 新增 **Admin 设置项** `oidc_provider.*`：`enabled`、`issuer_url`、`access_token_ttl_seconds`、`id_token_ttl_seconds`、`refresh_token_ttl_seconds`、`code_ttl_seconds`、`signing_key_active_kid`、`sso_cookie_domain`
- 新增 **Admin OIDC Client 管理页面**：列表 / 创建 / 编辑 / 删除 / 重置 secret / 启停；列表中支持查看每个 client 的近 30 天授权次数（不在本期实施则记入 Open Questions）
- 新增 **前端 Consent 页面** `/oidc/consent`：未授权 client 第一次登录或 scope 变化时展示授权同意页（取决于 client 配置 `consent_required`）

## Capabilities

### New Capabilities

- `oidc-provider`：sub2api 作为标准 OIDC 提供者对外提供 SSO 与 ID Token 签发能力。覆盖：Discovery、JWKS、Authorization Code + PKCE 流程、Refresh Token 轮换、UserInfo 与 scope claim 投影（含 `openid`/`profile`/`email`/`offline_access`/`sub2api:balance`/`sub2api:apikey`）、HttpOnly SSO Cookie 登录态识别、Consent 同意机制、RSA 签名密钥生命周期、客户端注册/吊销、Admin 配置面板、错误响应规范化（OIDC error/error_description）。本 capability 是新增，与现有 `captcha` / `pricing-plaza` / `recharge-bonus` / `console-navigation` 不重叠。

### Modified Capabilities

<!-- 现有 4 个 capability（captcha/pricing-plaza/recharge-bonus/console-navigation）的 spec 行为均不发生变化。
     登录流程在 set SSO cookie 这一动作上有侧向新增，但因为没有现存的"login"/"auth"
     capability 把登录行为正式化为 spec，所以无 delta；该侧向新增完全归在新 capability
     `oidc-provider` 内部描述（"什么时候 set sub2api SSO cookie"）。 -->

## Impact

- 后端代码（新增）：
  - `backend/internal/handler/oidc_provider_handler.go`：6 个 OIDC 端点
  - `backend/internal/handler/oidc_provider_consent_handler.go`：Consent GET/POST
  - `backend/internal/handler/admin/oidc_client_handler.go`：Admin CRUD
  - `backend/internal/service/oidc_provider_service.go`：业务编排（authorize / token / userinfo / refresh / revoke）
  - `backend/internal/service/oidc_signing_service.go`：RSA keypair 加载/生成/JWKS 投影/ID Token 签名
  - `backend/internal/service/oidc_client_service.go`：client 注册、secret hash 校验
  - `backend/internal/service/oidc_consent_service.go`：consent 读写
  - `backend/internal/service/sso_session_service.go`：HttpOnly cookie 签发、解析、吊销
  - `backend/internal/server/routes/oidc_provider.go`：路由注册
  - `backend/ent/schema/oidc_client.go`、`oidc_authorization_code.go`、`oidc_refresh_token.go`、`oidc_consent.go`、`sso_session.go`：5 个新 ent schema
- 后端代码（修改）：
  - [auth_service.go](/home/jiantaoli/sub2api/backend/internal/service/auth_service.go)：所有"用户登录成功"路径调用 `ssoSessionService.Issue(ctx, userID, w)` 顺带 set cookie；登出路径调用 `Revoke`
  - [setting_handler.go](/home/jiantaoli/sub2api/backend/internal/handler/admin/setting_handler.go)：注册 `oidc_provider.*` 8 项配置
  - 后端启动初始化（main / wire）：注入新 service、首次启动时调用 `oidcSigningService.EnsureActiveKey()` 自动生成 RSA-2048
- 前端代码（新增）：
  - `frontend/src/views/oidc/ConsentView.vue`：用户授权同意页
  - `frontend/src/views/admin/OidcClientsView.vue`：Admin client 管理列表 + 编辑抽屉
  - `frontend/src/api/oidcClients.ts`：Admin API 调用封装
- 前端代码（修改）：
  - `frontend/src/router/index.ts`：注册 `/oidc/consent`、`/admin/oidc-clients` 路由
  - `frontend/src/views/admin/SettingsView.vue`：新增"OIDC Provider"分组（issuer_url / TTL / 启停 / 主 kid 切换 / 公钥导出）
- 数据库迁移：新增 5 张表 + 1 个为 `security_secrets` 写入种子（active OIDC kid）的迁移；遵循 forward-only 约定（参见 [migrations/README.md](/home/jiantaoli/sub2api/backend/migrations/README.md)）
- 外部依赖：不引入新 Go 模块；RSA/JWT/JWKS 全部使用既有 `github.com/golang-jwt/jwt/v5` 与标准库 `crypto/rsa`、`crypto/rand`、`encoding/base64`
- 配置/部署：上线前 admin 必须填 `oidc_provider.issuer_url`（应为对外 HTTPS 地址，如 `https://api.sub2api.com`）和 `oidc_provider.enabled=true`，否则 6 个端点继续返回 404
- 风险：
  - SSO Cookie 跨子域共享需要正确配置 `sso_cookie_domain`，配置错误会让 SSO 失效但不影响主站登录
  - RSA 私钥落库 `security_secrets` 表，DB dump 等同泄露私钥；运维需把 `security_secrets` 表纳入备份加密策略（与 TOTP 加密密钥同级）
  - private scope `sub2api:balance` 一旦授权出去等于把"该用户的余额"开放给第三方应用读取，admin 创建 client 时 UI 必须给出红色警示
