## Context

sub2api 现状：

1. **认证后端**：`AuthService.GenerateTokenPair`（[auth_service.go:1432](/home/jiantaoli/sub2api/backend/internal/service/auth_service.go)）签发 access/refresh token，签名算法 `HS256`，密钥从 `cfg.JWT.Secret` 取。所有 API 都靠这套 token 鉴权，前端把 access token 放在 `localStorage`，请求时塞到 `Authorization` header。
2. **OIDC 客户端**：[auth_oidc_oauth.go](/home/jiantaoli/sub2api/backend/internal/handler/auth_oidc_oauth.go)（39KB，已实现）用于把 sub2api 接到外部 IdP（如 Authelia / Keycloak），与本变更无功能重叠，但**有现成的 OIDC 字段处理代码**（PKCE、ID Token 解析、JWKS 客户端拉取）可以反向参考。
3. **用户身份**：[ent/schema/user.go](/home/jiantaoli/sub2api/backend/ent/schema/user.go) 已有 `id`、`email`、`username`、`role`、`status`、`balance`(decimal 20,8)、`total_recharged` 等字段，可直接投影为 OIDC claims；`api_keys` 是 edge。
4. **密钥存储**：[security_secret.go](/home/jiantaoli/sub2api/backend/ent/schema/security_secret.go) 是为 JWT 签名密钥、TOTP 加密密钥这类系统级密钥设计的 KV 表（key + text value），完美承载 OIDC RSA 密钥对。
5. **动态配置**：[setting.go](/home/jiantaoli/sub2api/backend/ent/schema/setting.go) 是简单 KV，已经承载了大量 admin 可配置项；新增 `oidc_provider.*` 顺手。
6. **会话承载**：`pending_auth_sessions` 表语义被锁在 `login` / `bind_current_user` / `adopt_existing_user_by_email` 三种 intent，**不应**复用承载 OIDC authorization code 与 SSO 会话；新建独立 schema。
7. **路由模式**：handler 层目录约定 `auth_*.go` / `admin/*.go`；service 层 `*_service.go`。本变更新增文件遵循同名约定。

约束：

- 不引入新 Go 模块。OIDC 协议层用既有 `github.com/golang-jwt/jwt/v5` + `crypto/rsa` + 标准库。
- 现有 HS256 access token 流程**不变** —— sub2api 自身 API（如 `/api/v1/usage` 等）继续认这套 token，改了波及面太大。OIDC ID token 是另一套独立签发的 token，只签给第三方 RP 用。
- 现有 localStorage JWT 登录态**不变** —— 前端代码不需改 token 存放。新增的 SSO HttpOnly cookie 是在登录成功的服务端响应里**额外** set 一个，仅供 `/oidc/authorize` 浏览器跳转识别用。
- 数据库 forward-only 迁移（[migrations/README.md](/home/jiantaoli/sub2api/backend/migrations/README.md)）。
- Issuer URL 必须可对外解析；JWKS 必须任意客户端可匿名访问。
- ent schema 改动需要 `make generate` 或等价指令重新生成 ent 代码。
- 用户已确认的 7 项决策（Decision Log，引用自上轮对话）：
  1. 签名算法 = RSA-2048 / RS256（A 方案）
  2. Client 管理 = Admin 手工录入（A 方案）
  3. Grant Type = 仅 Authorization Code + PKCE + Refresh Token
  4. Consent = Admin 配置 client 是否需要 consent（C 方案）
  5. Scope = `openid` `profile` `email` `offline_access` + 私有 `sub2api:balance` `sub2api:apikey`
  6. 登录态 = 新增 HttpOnly SSO Cookie（A 方案）
  7. Issuer URL = Admin 配置（A 方案）

## Goals / Non-Goals

**Goals:**

- sub2api 通过 OpenID Connect Core 1.0 Conformance Profile 中"Basic OP"必需端点的合规校验（Discovery / JWKS / Authorize / Token / UserInfo）。
- 任何标准 OIDC 客户端（如 oidc-client-ts、go-oidc、appauth-android）只用 issuer URL 一个配置项就能完成接入。
- ID Token、Refresh Token、Authorization Code 三类 token/code 全部走独立短生命周期 + 一次性 + 可吊销路径，不和 sub2api 自身 access token 混用。
- 新增 SSO Cookie 不破坏现有"刷新页 → localStorage 取 token → 静默续期"的前端登录态恢复路径。
- 私钥永远不离开服务进程内存或 `security_secrets` 表；JWKS 端点只暴露公钥；支持手动滚动新 kid（旧 kid 仍可签验旧 token 直到过期）。
- 私有 scope `sub2api:balance` / `sub2api:apikey` 是显式 opt-in：admin 创建 client 时必须勾选并接受风险提示。

**Non-Goals:**

- **不**实现 RFC 7591 动态客户端注册（决策 2）。
- **不**实现 `implicit` / `hybrid` / `password` / `client_credentials` 这些 grant type（决策 3）。
- **不**实现 OIDC Logout（RP-Initiated Logout / Front-Channel / Back-Channel）；本期下线 client 由 admin 手动吊销 + refresh token 全家族吊销代替。
- **不**实现 Pairwise Subject Identifier；`sub` 直接用 sub2api 的 user_id（decimal 字符串）。
- **不**实现 ACR / AMR claim 的精细化（仅在用户用 TOTP 登录时输出 `amr: ["pwd","mfa"]`，否则 `["pwd"]`）。
- **不**实现 Request Object（`request` / `request_uri` 参数）。
- **不**实现 Pushed Authorization Requests（PAR）。
- **不**实现 `prompt=login`/`prompt=none` 的精细语义（仅识别，遇到 `prompt=login` 时强制重新登录；其他值忽略）。
- **不**改 sub2api 自身 API 的鉴权 —— 现 HS256 access token 流不变。
- **不**支持 ID Token Encryption / UserInfo Encryption；只支持签名（RS256）。
- **不**做 OIDC client 的"近 30 天授权次数"统计（移入 Open Questions）。

## Decisions

### D1：签名算法 = RS256 + RSA-2048（决策 1 落地）

- **What**: ID Token 使用 `RS256`（RSASSA-PKCS1-v1_5-SHA-256）+ 2048-bit RSA 密钥。Access Token（OIDC 端点签发的，用于 UserInfo）也用 RS256，与 sub2api 自身 access token（HS256）严格隔离。Discovery 文档 `id_token_signing_alg_values_supported: ["RS256"]`。
- **Why**:
  - OIDC 客户端生态对 RS256 最友好（go-oidc、oidc-client-ts 默认验证 RS）。
  - RSA-2048 是当前业界默认安全水位，密钥派生快（启动一次性），客户端验签也快。
  - 不选 ES256：椭圆曲线虽更快、密钥更短，但 jwt/v5 库对 ES 支持需要更小心的曲线/编码处理；RS256 出错面更小。
- **Alternatives considered**:
  - 继续用 HS256：客户端必须知道共享密钥，违反 OIDC 部署假设，否决。
  - ES256：未来可在新 kid 上启用（kid 切换机制本身已支持），本期不实施。

### D2：密钥存储与 kid 滚动 = 表 `security_secrets` + key 前缀 `oidc_provider.signing_key.<kid>`

- **What**:
  - 进程启动时调用 `OidcSigningService.EnsureActiveKey()`：
    1. 读 setting `oidc_provider.signing_key_active_kid`，若空 → 生成 RSA-2048，kid = `time.Now().UTC().Format("20060102T150405Z")`，把 PEM-encoded PKCS1 私钥写入 `security_secrets` 表（key=`oidc_provider.signing_key.<kid>`），同时写 setting `signing_key_active_kid`。
    2. 若不空 → 加载该 kid 的 PEM。
  - 内存里维护 `map[kid]*rsa.PrivateKey`，启动时一次性把 `security_secrets` 中所有 `oidc_provider.signing_key.*` 都加载（用于 JWKS 投影所有公钥）。
  - 滚动新 kid：admin 调一个 `POST /admin/oidc/signing-keys/rotate` → 新建 RSA-2048 + 切换 active_kid → 旧 kid 仍保留在 JWKS 中至少 7 天（让旧 ID Token 过期前仍可验签）。
  - 删除旧 kid：admin 显式调用 `DELETE /admin/oidc/signing-keys/<kid>`，前端 UI 禁用"删除当前 active"。
- **Why**:
  - 复用 `security_secrets` 表，避免另起一张几乎一样的表；它本来就为这种场景而设计（注释明确写"JWT 签名密钥、TOTP 加密密钥"）。
  - kid 用时间戳避免冲突且语义自解释（运维一眼看出哪把更新）。
  - 旧 kid 不立即删 → ID Token 默认 1 小时寿命，7 天足够覆盖任意客户端缓存。
- **Alternatives considered**:
  - 把私钥放配置文件 / 环境变量：每次部署运维要管 PEM 文件，初次部署摩擦大；多副本部署时一致性要靠外部协调（k8s secret 没问题但 docker-compose 不友好）。
  - 新建专用表 `oidc_signing_keys`：字段会和 `security_secrets` 几乎一致；不值得。

### D3：Client 注册 = ent schema `oidc_client` + Admin CRUD（决策 2 落地）

- **What**: 新增 schema：
  ```
  oidc_client:
    id (int64 PK)
    client_id (string, unique, MaxLen 64)               // 公开标识
    client_secret_hash (string, text)                    // bcrypt(secret)，永不存明文
    client_name (string, MaxLen 100)                     // 展示名
    redirect_uris (jsonb -> []string)                    // 严格匹配，不支持通配
    allowed_scopes (jsonb -> []string)                   // 该 client 允许申请哪些 scope
    grant_types (jsonb -> []string)                      // ["authorization_code","refresh_token"] 固定
    consent_required (bool, default true)                // 决策 4：admin 配置
    enabled (bool, default true)
    created_at / updated_at (TimeMixin)
  unique index: client_id
  ```
  - 创建 client：admin 提交 → 后端生成 `client_id = "rp_" + base32(rand 16B)`、`client_secret = base64url(rand 32B)`、`client_secret_hash = bcrypt(secret, cost=10)`、明文 secret **仅在创建响应中返回一次**，之后任何 GET 都不能拿到明文。
  - 重置 secret：生成新 secret + 更新 hash + 一次性返回。
  - 校验：token 端点取 client 凭证（支持 `client_secret_basic` 与 `client_secret_post` 两种 OAuth2 标准方式）→ bcrypt compare。
- **Why**:
  - bcrypt 防 DB 泄露下的暴力破解；secret 不长（256 bits 熵），bcrypt cost=10 性能足够（token 端点 QPS 极低，不可能成瓶颈）。
  - redirect_uris 严格相等匹配 → 杜绝 open redirect 类攻击。
  - 字段 `allowed_scopes` 限定 client 自身允许申请的 scope 子集，admin 必须显式勾选 `sub2api:balance` 才能申请该 scope，给敏感 scope 多一道闸。
- **Alternatives considered**:
  - 把 client 信息塞进 setting 表（jsonb 字段）：不利于按 client_id 索引，且 admin 列表分页/搜索很别扭。
  - 用 sha256(secret) 而非 bcrypt：sha256 抗暴力差；bcrypt 只在 token 端点用，QPS 低。

### D4：Grant Type 仅 `authorization_code` + `refresh_token` + 强制 PKCE（决策 3 落地）

- **What**:
  - `/oidc/authorize` 必须带 `code_challenge`（不带直接 400）；`code_challenge_method` 必须是 `S256`（不接受 `plain`）。
  - `/oidc/token` `grant_type=authorization_code`：必须带 `code_verifier`，与发码时存的 `code_challenge` 比对。
  - `/oidc/token` `grant_type=refresh_token`：refresh token 一次性 —— 一次成功兑换后，旧 refresh token 立即失效，签发新 refresh + access + (可选)新 ID token。
  - **Refresh Token Family Rotation（防盗用）**：每次 refresh 时所有同 family 的旧 refresh 全部失效；如果 server 收到一个已经被使用过的 refresh（`reused=true`），整个 family 立即吊销 + 标记为可疑。
- **Why**:
  - PKCE 是 OAuth 2.1 默认要求；公开客户端（SPA / Mobile）必需，机密客户端也加只是多算一次 hash，没坏处。
  - 一次性 + family rotation 是 OIDC 防 refresh token 盗用的标准做法（参考 OAuth 2.0 Security BCP）。
  - `S256` only：`plain` 没意义，移除一个攻击面。
- **Alternatives considered**:
  - 允许 refresh token 多次使用：盗用风险大，决策 3 已锁定不做。
  - 允许 `plain` PKCE：兼容性收益几乎 0，否决。

### D5：Consent = client 级开关 + 用户级记忆（决策 4 落地）

- **What**:
  - schema `oidc_consent`：`(user_id, client_id) → granted_scopes jsonb, granted_at, last_used_at`，唯一索引 `(user_id, client_id)`。
  - `/oidc/authorize` 流程：
    1. 解析 client、redirect_uri、scopes、PKCE 等参数 → 通过则继续，失败则按 OIDC 规范返回 error redirect。
    2. 检查 SSO Cookie → 未登录 → 302 跳到登录页（带 `next=/oidc/authorize?...`），登录完成后跳回。
    3. 已登录 → 加载 client 的 `consent_required`：
       - `false`（admin 信任的内部 client）：直接发 code 跳回 redirect_uri。
       - `true`：查 `oidc_consent` 是否已有覆盖本次 scope 的记录 → 有则直接发 code；无则 302 到 `/oidc/consent?session=<csrf-bound-token>` 让用户确认。
    4. 用户在 `/oidc/consent` 同意 → 写 `oidc_consent` 记录 → 服务端发 code → 跳回 redirect_uri。
    5. 用户拒绝 → 按 OIDC 规范返回 `error=access_denied` 重定向。
  - "覆盖本次 scope" = 已 granted 的 scopes 是 ⊇ 本次请求 scopes 的超集；否则增量 scope 走重新 consent。
- **Why**:
  - admin 信任的纯内部 client（如 sub2api 自家的子站）跳过 consent，UX 不刺眼。
  - 第三方 client 强制 consent + 记忆同意 → 标准做法。
  - `granted_scopes` 整体覆盖判断比"逐 scope 标 granted"简单 1 个数量级，且语义足够（若客户端要扩 scope 必然重写授权请求）。
- **Alternatives considered**:
  - 全局开关（4A：第一次必弹）：内部 SSO 场景多余，否决。
  - 完全不弹（4B）：第三方场景下用户无知情权，否决。

### D6：SSO Cookie 设计（决策 6 落地）

- **What**:
  - 表 `sso_session`：
    ```
    sso_session:
      id (int64 PK)
      session_id (string, unique, 32B base64url)         // 即 cookie value
      user_id (int64 FK -> users.id, on delete cascade)
      issued_at, last_seen_at, expires_at (timestamptz)
      revoked_at (timestamptz, nullable)
      user_agent (text, default '')                       // 仅用于 admin 审计
      ip_address (text, default '')                       // 仅用于 admin 审计
    indexes: session_id unique, user_id, expires_at
    ```
  - Cookie 名：`sub2api_sso`；Attributes：`HttpOnly; Secure; SameSite=Lax; Path=/; Max-Age=<configurable, default 30d>; Domain=<configurable>`.
  - `SsoSessionService.Issue(ctx, userID, w, r)` 在所有"用户登录刚成功"的代码路径调用：
    - 密码登录：`auth_handler.go` 的 `Login` 成功分支
    - 邮箱魔法链接：`auth_email_oauth.go` 完成分支
    - 外部 OAuth（OIDC client / 钉钉 / 企微 / Linux.do / 飞书）完成分支
    - 注册成功后自动登录的分支
  - 登出（`POST /api/v1/auth/logout`）调用 `Revoke(ctx, sessionID, w)` → 写 `revoked_at` + `Set-Cookie: sub2api_sso=; Max-Age=0`。
  - `/oidc/authorize` 解析 cookie：未带或解析失败 → 视为未登录；已 revoke / 已过期 → 视为未登录；有效 → 取 user_id 作为身份。
  - **延长 last_seen_at**：每次 `/oidc/authorize` 命中时异步更新（不阻塞响应）。
- **Why**:
  - 不能复用前端 localStorage 的 access token：浏览器跨域 GET 跳转拿不到 localStorage。
  - 不复用 access token 写 cookie：access token 1 小时寿命，每小时让用户重登录不可接受；且 access token 是 HS256，签验机制混用容易出事。
  - 独立 sso_session 表 → 服务端可吊销（admin 一键踢人）、可审计、独立 TTL。
  - SameSite=Lax：top-level GET 跳转能带（OIDC `/authorize` 正是这种场景）；XHR 跨站攻击不带，CSRF 风险可控。
  - Domain 可配置：sub2api 主站和 OIDC 端点在同域时无需配；跨子域（`auth.sub2api.com` vs `app.sub2api.com`）时配 `.sub2api.com`。
- **Alternatives considered**:
  - 把 access token 同步写 HttpOnly cookie：寿命短、签算法不匹配、轮询/续期复杂，否决。
  - 跳到登录页让用户每次重登（决策 6 选项 B）：单点登录变成"每次都登"，体验差，否决。

### D7：Authorization Code 与 Refresh Token 各自独立表

- **What**:
  ```
  oidc_authorization_code:
    id (int64 PK)
    code (string, unique, 32B base64url)
    client_id (string)                                   // 业务 client_id 字符串值
    user_id (int64)
    redirect_uri (text)                                  // 必须与发码请求一致
    scopes (jsonb -> []string)
    code_challenge (string)
    code_challenge_method (string)                       // 固定 "S256"
    nonce (text, default '')                             // 透传到 ID Token
    issued_at, expires_at (timestamptz)                  // 默认 10 分钟
    consumed_at (timestamptz, nullable)                  // 一次性
  indexes: code unique, expires_at, user_id

  oidc_refresh_token:
    id (int64 PK)
    token (string, unique, 32B base64url)                // opaque，不是 JWT
    family_id (string, MaxLen 64)                        // 同一次授权链上的所有 refresh 共享
    client_id, user_id
    scopes (jsonb -> []string)
    issued_at, expires_at (timestamptz)                  // 默认 30 天
    revoked_at (timestamptz, nullable)
    parent_token_hash (string, default '')               // 上一代的 token hash，便于审计
  indexes: token unique, family_id, user_id, expires_at
  ```
- **Why**:
  - code 和 refresh 语义不同（code 极短、redirect_uri 强绑、PKCE 强绑；refresh 长 + family + scope）；混在一张表会让所有列大半为空。
  - refresh token 用 opaque 而非 JWT：
    - 可吊销（JWT 无法撤销，只能等过期）；
    - 不需要客户端做任何额外解析（refresh 只发回 token endpoint）；
    - DB 查 token unique 索引一次查到，<1ms。
- **Alternatives considered**:
  - refresh 用 JWT：吊销难，否决。
  - 复用 `pending_auth_sessions`：intent 字段已被锁定不允许 OIDC 用，强行扩 intent 会牵连 4 处现有逻辑测试，否决。

### D8：Scope 与 Claim 投影

- **What**: scope → claims 映射严格如下：

  | scope | id_token claims | userinfo claims |
  |---|---|---|
  | `openid` (强制) | `sub`, `iss`, `aud`, `exp`, `iat`, `auth_time`, `nonce`, `acr`, `amr` | `sub` |
  | `profile` | `name`, `preferred_username`, `updated_at` | 同 id_token |
  | `email` | `email`, `email_verified` | 同 id_token |
  | `offline_access` | （无 claim，触发签发 refresh_token） | — |
  | `sub2api:balance` | （不放 id_token，避免 token 体积） | `sub2api_balance`（string，decimal）, `sub2api_total_recharged`（string） |
  | `sub2api:apikey` | （不放 id_token） | `sub2api_apikey_count`（int） |

  - `email_verified`：sub2api 的注册邮箱 = 已验证（注册流强制邮箱验证），统一返回 `true`。如果未来引入未验证注册路径，按实际状态来。
  - `name` / `preferred_username`：从 user.username 取；username 为空则回退 email 的 local-part。
  - `acr` 固定 `"urn:sub2api:authn:basic"`（仅密码登录）或 `"urn:sub2api:authn:mfa"`（TOTP 完成）。
  - `auth_time`：用户最近一次完成首因素登录的 unix 时间（来自 `sso_session.issued_at`，MFA 重验也算更新）。
  - **私钥级别风险红字**：`sub2api:balance` 暴露财务信息、`sub2api:apikey_count` 间接暴露 API 使用强度。Admin 创建 client 勾选这俩 scope 时 UI 必须红色警示且要求二次确认。
- **Why**:
  - id_token 不放 balance / apikey_count → 避免每次签发都 hit decimal 序列化、避免静态 token 反复曝光实时数值。
  - userinfo 才是 OIDC 标准上"实时拉用户最新画像"的入口，把动态数值放这里语义正确。
  - 不暴露明文 api key：决策与"OIDC 提供身份信息、不充当 secret 分发器"的语义一致；如果第三方需要 sub2api API 调用能力，应单独走 sub2api 自身 API key 创建流程。
- **Alternatives considered**:
  - 把所有私有信息塞 id_token：方便客户端但 token 膨胀且暴露面大，否决。
  - 把明文 api_key 也吐出去：决策 5 你说的是 `apikey`（语义偏 metadata 而非 secret），且对 OIDC 不合适，否决。

### D9：Issuer URL 与 Discovery（决策 7 落地）

- **What**:
  - Setting `oidc_provider.issuer_url`（必填，启动前/admin 先写）。值形如 `https://api.sub2api.com`，末尾**不**带斜杠。
  - Discovery 文档生成时所有端点都用 `issuer_url` 作前缀拼出来：
    ```
    issuer:                          {issuer_url}
    authorization_endpoint:          {issuer_url}/oidc/authorize
    token_endpoint:                  {issuer_url}/oidc/token
    userinfo_endpoint:               {issuer_url}/oidc/userinfo
    jwks_uri:                        {issuer_url}/.well-known/jwks.json
    ```
  - id_token `iss` claim 取此值。
  - 启动校验：`oidc_provider.enabled=true` 但 `issuer_url` 为空 → 启动失败 panic（不允许启用却没填 issuer）。
- **Why**:
  - 决策 7 里讲过：自动从 `c.Request.Host` 推断在反代/k8s ingress 下不稳定。
  - 写死格式约束（无尾斜杠）让客户端 discovery URL 拼接零分歧。
- **Alternatives considered**:
  - `c.Request.Host` 推断：决策 7 明确否决。

### D10：错误响应规范化

- **What**:
  - `/oidc/authorize` 错误：能 redirect 就 302 redirect 到 `redirect_uri?error=<code>&error_description=<...>&state=<state>`；不能 redirect（如 client_id 无效或 redirect_uri 无效）→ 400 + JSON `{error, error_description}`。
  - `/oidc/token` / `/oidc/userinfo` 错误：HTTP 4xx + JSON `{error, error_description}`。
  - 错误码使用 OIDC/OAuth2 标准集合：`invalid_request`、`invalid_client`、`invalid_grant`、`unauthorized_client`、`unsupported_grant_type`、`invalid_scope`、`access_denied`、`server_error`。
  - **不**透出 sub2api 内部错误细节（DB 错、PEM 解析错等）；这些归 `server_error` + 详细日志带 trace id。
- **Why**: OIDC 规范严格要求；客户端 SDK 也按这套错误码做分支。
- **Alternatives considered**:
  - 用 sub2api 自家 ResponseEnvelope：客户端 SDK 不认，否决。

### D11：路由与组装

- **What**:
  - 新建 `backend/internal/server/routes/oidc_provider.go`，挂载在主 router（不在 admin group / 不在 user-auth group），无前缀：
    ```
    GET  /.well-known/openid-configuration
    GET  /.well-known/jwks.json
    GET  /oidc/authorize
    POST /oidc/token
    GET  /oidc/userinfo
    GET  /oidc/consent
    POST /oidc/consent
    ```
  - admin 路由：
    ```
    GET    /api/v1/admin/oidc/clients
    POST   /api/v1/admin/oidc/clients
    PATCH  /api/v1/admin/oidc/clients/:id
    DELETE /api/v1/admin/oidc/clients/:id
    POST   /api/v1/admin/oidc/clients/:id/reset-secret
    POST   /api/v1/admin/oidc/signing-keys/rotate
    DELETE /api/v1/admin/oidc/signing-keys/:kid
    GET    /api/v1/admin/oidc/signing-keys
    ```
  - 当 `oidc_provider.enabled=false` 时，`/oidc/*` 与 `/.well-known/openid-configuration` 一律 404（不暴露能力）。jwks 端点行为同步 404。
- **Why**:
  - well-known 是 OIDC 强制路径，不能随便加前缀。
  - admin 走 `/api/v1/admin/*` 与既有 admin 路由对齐。
  - enabled=false 时 404 而非 200 空响应：避免被探测出"这是个 sub2api 实例"。

### D12：测试策略

- **What**:
  - 单测：每个 service 都写。重点：
    - `oidc_signing_service`：密钥生成幂等、JWKS 投影 modulus/exponent 正确、签出的 ID Token 用导出的 JWKS 验签通过、kid 滚动后旧 kid 仍可验旧 token。
    - `oidc_provider_service`：authorize 各种参数错误的分支；code 一次性；refresh family rotation；reuse detection。
    - `oidc_consent_service`：scope superset 判断。
    - `sso_session_service`：cookie 签发、解析、吊销。
  - 集成测试：开 gin testserver，跑完整 authorization code + PKCE 流，用 `go-oidc` 当客户端验签 ID Token 走通。
  - Conformance 自查清单（不在 CI 里跑，手动）：用 oidcbox / openid certified 工具手测。
- **Why**: OIDC 协议层错误代价高（一旦发出去的 ID Token 客户端验签失败，没法补救），单元 + 集成 + 第三方验签三层兜底。

### D13：迁移与 ent 代码生成

- **What**:
  - 5 个新 ent schema 文件：`oidc_client.go`、`oidc_authorization_code.go`、`oidc_refresh_token.go`、`oidc_consent.go`、`sso_session.go`。
  - 5 张表对应一个 forward-only 迁移文件：`backend/migrations/<NNN>_add_oidc_provider_tables.sql`，编号取当前最大 +1。
  - `make generate` 重生成 ent client 代码。
  - 不需要 down migration（仓库约定）。
- **Why**: 与项目既有迁移规范完全对齐。

## Risks / Trade-offs

- **私钥落 DB → DB dump 等同泄露密钥** → Mitigation：(1) `security_secrets` 表本来就放 TOTP 加密密钥等高敏数据，运维已知该表需要加密备份；(2) 在 admin 设置页"OIDC Provider"模块顶部加红字提醒；(3) 提供 kid rotate 操作，万一怀疑泄露能立刻滚密钥。
- **`sub2api:balance` scope 一旦授权出去 = 余额对第三方可见** → Mitigation：(1) admin 创建 client 勾选该 scope 时弹红色二次确认；(2) consent 页面强制展示该 scope 的人话描述（"读取您的账户余额"）；(3) 用户可在 `/console/oidc-authorizations`（后续 follow-up，不在本期）查看并撤销已授权的 client。
- **SSO Cookie 跨子域配置错误 → SSO 失效** → Mitigation：(1) admin 设置项 `sso_cookie_domain` 留空时不下发 Domain 属性（仅当前精确域名生效，最安全的默认值）；(2) admin 设置页面对此字段加示例 `.sub2api.com` 与说明。
- **ID Token 体积**：profile + email scope 下 ID Token ~1KB，URL fragment 模式不支持（决策 3 不允许 implicit 也无所谓），但 Cookie/Header 携带没问题。
- **/oidc/authorize 高并发下写 sso_session.last_seen_at** → Mitigation：异步 + 限流（每个 session 最多 1 分钟更新一次 last_seen_at）。
- **Client secret 创建后只显示一次** → 用户体验摩擦，但是安全上必须如此 → Mitigation：UI 创建成功页提供"复制到剪贴板"按钮 + 红色提示"此 secret 不会再显示"。
- **Refresh token 一次性 + family rotation 在网络重试场景下误杀** → 客户端 SDK（如 go-oidc）一般做了"已经成功一次就缓存新 token"，问题不大；但纯前端 SDK 如果在拿到响应前用户刷了页可能丢新 token 触发同 family 复用 → 表现为该用户被强制重新走 authorize 一次（不是大事故）。
- **issuer_url 错填 → discovery 文档拼出错误端点 → 客户端无法 token 兑换** → Mitigation：admin 设置页对该字段加格式校验（必须 https://、必须无尾斜杠）+ 启动时和 self-test：调用本机 `/.well-known/openid-configuration` 确认能解析。
- **私有 scope 命名 `sub2api:balance`（含冒号）在某些 OIDC 客户端解析空格分割时无问题；但 OIDC scope 严格规范要求 scope token = `1*( %x21 / %x23-5B / %x5D-7E )`，冒号 `%x3A` 在允许范围内** → 安全。

## Migration Plan

1. **后端发版（不开启功能）**：5 个 ent schema 落库 + service/handler 代码合并 + admin 路由注册；setting `oidc_provider.enabled` 默认 `false`。CI 必须包含 D12 列出的所有单测 + 集成测试。
2. **首次启用配置**：admin 在后台进入"OIDC Provider"页 →
   - 填 `issuer_url`、`sso_cookie_domain`（可选）、TTL 三项（可用默认）
   - 点击"生成签名密钥" → 后端生成 RSA-2048 写入 `security_secrets`
   - 切换 `enabled=true`
   - 自检：访问 `https://<issuer>/.well-known/openid-configuration` 应返回 200 + 合法 JSON
3. **创建第一个 client**：admin 创建一个测试 client（consent_required=true，scopes=`openid profile email`）→ 用 `oidc-client-tools` 跑通登录回跳 → 拿到 ID Token → 用 jwks 验签通过。
4. **生产灰度**：邀请 1～2 个内部应用先接入；观察 1 周。
5. **文档**：admin 后台"OIDC Provider"页面附第三方接入指南（issuer URL、各端点路径、scope 列表、redirect_uri 注册要求、私有 scope 风险说明）。

**Rollback**：

- 任何阶段发现协议合规性问题：admin 后台 `oidc_provider.enabled=false` → 6 个端点立刻返回 404，不需要回滚代码。
- 发现 schema 设计问题：因为是新增 5 张表，不影响现有功能，可以保留代码继续迭代下个 change 修正字段（forward-only 加列）。
- 怀疑密钥泄露：admin 一键 rotate kid + 立刻删旧 kid（接受所有已签发 ID Token 立即失效）。
- 怀疑某 client 泄露：admin 删除该 client 行（cascade 删除其所有 oidc_consent / refresh_token）。

## Open Questions

- **Q1**: 是否在本期实现"用户自助管理已授权 client 列表"页面（`/console/oidc-authorizations`）？目前默认 **不**做，作为 follow-up change。如果用户量级 > 100 个 OIDC client 时再补。
- **Q2**: 私有 scope 命名是 `sub2api:balance` / `sub2api:apikey`，定下来后将来很难改（已发出去的 client 还在用）。是否要采用 URL 风格（`https://sub2api.com/scopes/balance`）？默认 **不**采用，理由：URL 风格虽更规范但客户端配置繁琐、scope 字符串长。如果后续有跨组织联邦需求再迁移。
- **Q3**: client 列表的"近 30 天授权次数"统计要不要在本期做？默认 **不做**（需要新建 `oidc_authorize_event` 表 + cron 聚合，性价比低）。先记入运维需求，下个 change 再说。
- **Q4**: `/oidc/authorize` 收到 `prompt=login` 的语义是"无视 SSO Cookie 强制重新登录"，是否要支持？默认 **支持**（用户体验更标准），实现成本是 1 行 if 判断。
- **Q5**: Refresh Token 是否支持 `scope` downgrade（兑换时减少 scope）？OIDC 规范允许；默认 **支持**（解析 token 端点的 `scope` 参数，必须是原 refresh 携带 scope 的子集），实现简单。
- **Q6**: ID Token 是否需要包含 `groups` claim（基于 `user_allowed_groups` edge）？目前 user.role 已映射到 `acr`，群组可作为 `profile` scope 的扩展返回。本期 **不做**，避免决策面继续扩大；如果有内部 client 真有需求，下期再加。
