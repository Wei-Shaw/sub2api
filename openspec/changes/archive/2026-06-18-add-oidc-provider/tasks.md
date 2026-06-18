> **Stage 1A 完成**: 任务 1.1–1.7 + 2.1–2.8 + 5.1–5.4 已落地；6 张表 schema、ent 代码已重生成、migration `150_add_oidc_provider_tables.sql` 就绪、签名服务 11 个单测全过、Consent 服务 9 个单测全过、`go vet ./...` 干净。
>
> **Stage 1B-1 完成 (本会话)**: 任务 3.1–3.2 + 3.6 (SSO Session 服务 + 单测，**未接入登录路径**) + 4.1–4.6 + 4.9 (OIDC Client 服务 + 单测，**未写 admin handler/路由**) + 8.1 部分 (新增 `oidc_provider_settings.go` 集中 8 个 setting key 常量、默认值、`ValidateOidcIssuerURL` 严格校验函数)。新增代码量:`oidc_provider_settings.go` (~150 行) + `sso_session_service.go` (~430 行) + `oidc_client_service.go` (~470 行) + 两份单测约 600 行；OIDC 全套单测共 ~52 个全部通过 (新增 SSO Session 16 个 + OIDC Client 13 个 + ValidateOidcIssuerURL 1 个，叠加 1A 的 11+9 个)；`go vet ./...` 全仓库干净。**剩余高风险/紧耦合任务推到 Stage 1B-2**:任务 3.3–3.5 (改 6 处登录路径接 SSO cookie) / 4.7–4.10 (admin handler+路由+handler 测试) / 6.x (Provider 核心服务) / 7.x (HTTP handler+routes) / 8.2–8.4 (setting handler 改、signing-key admin handler、审计日志)。
>
> 决策更新: 决策 B1 — 新增独立的 `oidc_access_token` schema/表 (与 `oidc_refresh_token` 不共表)，对应任务 6.5 的"独立表方案"已锁定，不再采用 `kind` 列共表方案。

## 1. Database Schemas & Migration

- [x] 1.1 Add ent schema `backend/ent/schema/oidc_client.go` with fields per design D3 (client_id unique, client_secret_hash, client_name, redirect_uris jsonb, allowed_scopes jsonb, grant_types jsonb, consent_required, enabled, TimeMixin)
- [x] 1.2 Add ent schema `backend/ent/schema/oidc_authorization_code.go` with fields per design D7 (code unique, client_id, user_id, redirect_uri, scopes jsonb, code_challenge, code_challenge_method, nonce, expires_at, consumed_at)
- [x] 1.3 Add ent schema `backend/ent/schema/oidc_refresh_token.go` with fields per design D7 (token unique, family_id, client_id, user_id, scopes jsonb, expires_at, revoked_at, parent_token_hash)
- [x] 1.3.1 (新增, 决策 B1) Add ent schema `backend/ent/schema/oidc_access_token.go` 独立访问令牌表 (token unique, client_id, user_id, scopes jsonb, refresh_family_id, expires_at, revoked_at)
- [x] 1.4 Add ent schema `backend/ent/schema/oidc_consent.go` with unique index `(user_id, client_id)` and `granted_scopes jsonb`, `granted_at`, `last_used_at`
- [x] 1.5 Add ent schema `backend/ent/schema/sso_session.go` with fields per design D6 (session_id unique, user_id FK cascade, issued_at, last_seen_at, expires_at, revoked_at, totp_verified_at, user_agent, ip_address) + 同步在 `user.go` 加 `sso_sessions` edge cascade
- [x] 1.6 Run ent codegen (`go generate ./ent`) 已执行；`oidcclient/oidcaccesstoken/oidcauthorizationcode/oidcrefreshtoken/oidcconsent/ssosession` 6 个 ent 子包已生成并通过 `go build ./ent/...`
- [x] 1.7 Forward-only SQL migration `backend/migrations/150_add_oidc_provider_tables.sql` 已写入；建 6 张表 + 索引；`sso_sessions.user_id` 显式 `REFERENCES users(id) ON DELETE CASCADE`
- [ ] 1.8 Verify migration applies cleanly on a fresh DB and on a copy of the latest dev DB (待 staging 部署阶段执行)

## 2. Signing Key Service

- [x] 2.1 Create `backend/internal/service/oidc_signing_service.go` 暴露 `EnsureActiveKey`, `ActiveKid`, `VerificationKey(kid)`, `JWKS()`, `SignIDToken`, `RotateKey`, `DeleteKey`, `ListKeys`。
- [x] 2.2 Implement `EnsureActiveKey`: 读 setting `oidc_provider.signing_key_active_kid`; 空 → 生成 RSA-2048 via `crypto/rand.Reader`、PKCS#1 PEM 编码、写 `security_secrets` (key=`oidc_provider.signing_key.<timestamp-kid>`)、持久化 active_kid setting; 非空 → 复用既有行。
- [x] 2.3 启动时批量加载：扫描 `security_secrets` 中前缀 `oidc_provider.signing_key.` 的所有行，填充内存 `map[kid]*rsa.PrivateKey`，供 JWKS 与跨 kid 验签使用。
- [x] 2.4 实现 `RotateKey`：生成新 kid + 写 `security_secrets` 新行 + Set active_kid setting + 记录上一代 kid 退役 unix 秒到 `oidc_provider.signing_key.retired.<oldkid>` 设置项 (本实现按"宽松一致性"做：rotate 不强制单 DB 事务，admin 极低频操作；失败可手工核对，启动会以现存 active_kid 继续工作)。
- [x] 2.5 实现 `DeleteKey`：拒绝当前 active kid (返回 `ErrOidcSigningActiveKeyDeletion`)；其它 kid 删 `security_secrets` 行 + 删退役时间戳 setting + 内存清理。
- [x] 2.6 实现 `JWKS()`：遍历内存 keys map，过滤掉退役超 7 天的 kid，输出 `[]map[string]any` (`kty`/`kid`/`use`/`alg`/`n`/`e`)，`n`/`e` 用 base64url no-padding 编码 RSA modulus 与 exponent。永不导出 `d`/`p`/`q`/`dp`/`dq`/`qi`。
- [x] 2.7 实现 `SignIDToken(claims jwt.MapClaims)`：用 `jwt.SigningMethodRS256` 签名，header.kid = active kid。
- [x] 2.8 单元测试 11 项全过 (`TestOidcSigning_*`)：`EnsureActiveKey` 首次生成 + 重启幂等、`SignIDToken` 与 JWKS 互验、JWKS 不泄露私钥、Rotate 后旧 token 仍可验证、超宽限期旧 kid 被过滤、`DeleteKey` 拒绝 active、`DeleteKey` 删除非 active、`ListKeys` 报告 active/retired/removable、`bigIntExponentBytes` 标准 65537 编码。

## 3. SSO Session Service & Cookie Wiring

- [x] 3.1 Create `backend/internal/service/sso_session_service.go` with `Issue(ctx, w http.ResponseWriter, r *http.Request, userID int64) error`, `Resolve(ctx, r) (userID int64, sessionID string, ok bool)`, `Revoke(ctx, w, sessionID string) error`, `RevokeAllForUser(ctx, userID) error`, `TouchLastSeen(ctx, sessionID)` (async)
- [x] 3.2 Cookie attributes: name `sub2api_sso`, value 32-byte base64url random, `HttpOnly`, `Secure`, `SameSite=Lax`, `Path=/`, `Max-Age` from setting `oidc_provider.sso_cookie_max_age_seconds` default 2592000, `Domain` from setting `oidc_provider.sso_cookie_domain` if non-empty
- [x] 3.3 (Stage 1B-2 完成) 登录成功路径统一接入 best-effort、feature-gated 的 `IssueIfProviderEnabled`：在 `auth_handler.go` 的 `respondWithTokenPair` (password/register/2FA)、`auth_oauth_pending_flow.go` 的 3 处 OAuth funnel、`auth_email_oauth.go` 1 处共 5 个登录成功点新增 `issueSsoSession`；当 `oidc_provider.enabled != true` 时为 no-op，保证特性关闭时对既有登录零行为影响
- [x] 3.4 (Stage 1B-2 完成) Logout 与 RevokeAllSessions 接入 `revokeSsoSession`：读取 `sub2api_sso` cookie 调 `Revoke` 并下发 `Max-Age=0` 清除 cookie；忽略 `ErrSsoSessionNotFound`
- [x] 3.5 Implement `TouchLastSeen` with rate limiting (no DB write if last update was within the last 60s) and run from a goroutine so it never blocks `/oidc/authorize`
- [x] 3.6 Unit tests: Issue produces correct cookie attributes per setting combinations; Resolve handles missing/expired/revoked sessions correctly; Revoke ends both DB row and cookie

## 4. OIDC Client Service & Admin CRUD

- [x] 4.1 Create `backend/internal/service/oidc_client_service.go` with `Create(req) (client *OidcClient, plaintextSecret string, err error)`, `List(filters)`, `Get(id)`, `Update(id, patch)`, `Delete(id)`, `ResetSecret(id) (plaintextSecret string, err error)`, `Authenticate(ctx, clientID, presentedSecret) (*OidcClient, error)`
- [x] 4.2 Implement secret generation: 32 bytes from `crypto/rand`, base64url encode no padding; `client_id` = `"rp_" + base32(rand 16B without padding lowercase)`
- [x] 4.3 Implement `Authenticate` using `bcrypt.CompareHashAndPassword`; return distinct error sentinels for unknown client vs. wrong secret (handler converts both to `invalid_client` to avoid enumeration)
- [x] 4.4 Validate redirect_uris on Create/Update: must be non-empty array, every entry parseable URL with `https://` scheme (allow `http://localhost` for dev), no trailing slash mismatch (store exactly as provided)
- [x] 4.5 Validate allowed_scopes is subset of `["openid","profile","email","offline_access","sub2api:balance","sub2api:apikey"]`
- [x] 4.6 Implement cascade delete: Delete must remove all `oidc_consent`, `oidc_authorization_code`, `oidc_refresh_token` rows referencing `client_id` in the same transaction (本实现额外把 `oidc_access_token` 也纳入级联，符合决策 B1 的独立表方案)
- [x] 4.7 (Stage 1B-2 完成) Create `backend/internal/handler/admin/oidc_client_handler.go` with handlers for the 6 admin routes (List, Create, Get, Patch, Delete, ResetSecret)；错误经 `mapOidcClientError` 映射为 404/400；Create/ResetSecret 仅在响应体里一次性返回明文 secret
- [x] 4.8 (Stage 1B-2 完成) Register routes under `/api/v1/admin/oidc/clients` group with admin auth middleware (见 `routes/admin.go` 的 `registerOidcAdminRoutes`)
- [x] 4.9 Unit tests for service: secret round-trip with bcrypt, redirect_uri validation, allowed_scopes subset enforcement, cascade delete, ResetSecret invalidates the old secret
- [x] 4.10 (Stage 1B-3 完成) `oidc_client_handler_test.go`：Create 201 响应体含 `client_secret`、随后 Get/List 均不含；ResetSecret 返回与原值不同的新 secret；非法 scope→400、未知 id→404。全部通过

## 5. OIDC Consent Service

- [x] 5.1 Create `backend/internal/service/oidc_consent_service.go` 暴露 `LoadGrantedScopes`、`Grant`、`Revoke`、`TouchLastUsed`、`IsCovered`。
- [x] 5.2 `IsCovered` 严格 superset 判断：空 requested → true；空 granted + 非空 requested → false；忽略顺序与重复元素。
- [x] 5.3 `Grant` upsert：行存在则 `granted_scopes = 旧 ∪ 新` 并刷新 `last_used_at`；行不存在则插入新行；`TouchLastUsed` 仅刷 `last_used_at` 不动 scope，供 authorize 命中 superset 时使用。
- [x] 5.4 单测 9 项全过 (`TestOidcConsent_*` + `TestUnionStrings_*`)：未命中 / 插入新行 / 增量并集 / 同一请求内部去重 / Revoke 行 + 二次 Revoke 报错 / TouchLastUsed 不存在行报错 / IsCovered 全场景 / unionStrings 顺序与去重。

## 6. OIDC Provider Core Service

- [x] 6.1 (Stage 1B-2 完成) Create `backend/internal/service/oidc_provider_service.go` with high-level methods (Discovery/HandleAuthorize/ExchangeCode/RefreshToken/RevokeFamily/BuildUserInfo + consent token 签发/解码 + IssueCode/RecordConsent/LookupClient 等)
- [x] 6.2 (Stage 1B-2 完成) Implement authorize parameter validation per spec scenarios (response_type, scope subset, redirect_uri exact match, PKCE S256 required, prompt=login handling, client.enabled check)
- [x] 6.3 (Stage 1B-2 完成) Implement opaque code generation: 32 bytes from `crypto/rand`, base64url no padding; persist with code_ttl_seconds
- [x] 6.4 (Stage 1B-2 完成) Implement `ExchangeCode`: load code, check unconsumed/unexpired, validate redirect_uri/PKCE, mark consumed atomically, generate access_token and (if offline_access) refresh_token, sign ID Token via `oidcSigningService.SignIDToken`
- [x] 6.5 (Stage 1B-2 完成，决策 B1) Implement access token storage: opaque tokens persist in NEW table `oidc_access_token` (独立表方案)
- [x] 6.6 (Stage 1B-2 完成) Implement `RefreshToken`: load by token, ensure not revoked/expired, atomically mark revoked + insert new refresh in same family, support optional scope downgrade (must be subset)
- [x] 6.7 (Stage 1B-2 完成) Implement reuse detection: if the presented refresh token is already revoked, call `RevokeFamily` and return `invalid_grant`; emit security log
- [x] 6.8 (Stage 1B-2 完成) Implement claim assembly per Scope-to-Claim Mapping requirement (D8): switch by scope, never put balance/apikey_count in id_token
- [x] 6.9 (Stage 1B-2 完成) Implement `BuildUserInfo`: lookup access token row, load user, project claims based on token's stored scopes
- [x] 6.10 (Stage 1B-2 完成) Implement `acr` / `amr` derivation from session (minimal-touch path)
- [x] 6.11 (Stage 1B-3 完成) `oidc_provider_service_test.go` 覆盖 spec 各场景：ValidateAuthorize (unknown/disabled client、redirect 不匹配不可回跳、缺 PKCE、非 S256、scope 缺 openid/越权、unsupported response_type)、ExchangeCode (成功签发 access/refresh/id、无 offline_access 不发 refresh、单次使用 + code 复用吊销 family、redirect/PKCE 不匹配、unknown/wrong-client)、RefreshToken (轮转吊销旧 token、reuse 触发整 family 吊销、scope 降权 OK、越权拒绝、unknown)、BuildUserInfo (scope 投影、balance/apikey 私有 scope、缺/未知 token 401)、Consent token round-trip + superset bypass。全部通过

## 7. OIDC HTTP Handlers

- [x] 7.1 (Stage 1B-2 完成) Create `backend/internal/handler/oidc_provider_handler.go` with: `Discovery`, `JWKS`, `Authorize`, `Token`, `UserInfo`
- [x] 7.2 (Stage 1B-2 完成) Implement `Discovery` returning JSON per the Discovery requirement; 404 when `oidc_provider.enabled=false`
- [x] 7.3 (Stage 1B-2 完成) Implement `JWKS` returning `oidcSigningService.JWKS()` result; 404 when disabled
- [x] 7.4 (Stage 1B-2 完成) Implement `Authorize` GET: parse query params, call provider service, on success redirect to consent page (`/oauth/consent?consent=`) or directly to `redirect_uri` with `code`; on error redirect-vs-JSON branching per spec
- [x] 7.5 (Stage 1B-2 完成) Implement `Token` POST: support both `client_secret_basic` 与 `client_secret_post`; set `Cache-Control: no-store` 与 `Pragma: no-cache`
- [x] 7.6 (Stage 1B-2 完成) Implement `UserInfo` GET/POST: parse Bearer header, call `BuildUserInfo`, on error return `WWW-Authenticate: Bearer error="invalid_token"`
- [x] 7.7 (Stage 1B-2 完成) Create `backend/internal/handler/oidc_provider_consent_handler.go` with `ConsentGet`, `ConsentPost` (consent token 绑定 user_id + SSO 比对防 CSRF)
- [x] 7.8 (Stage 1B-2 完成) Create `backend/internal/server/routes/oidc_provider.go` registering the OIDC endpoints + `/.well-known/openid-configuration` + `/.well-known/jwks.json` at root router (no API version prefix)
- [x] 7.9 (Stage 1B-2 完成) Add startup validation when `oidc_provider.enabled=true` but `issuer_url` is empty
- [x] 7.10 (Stage 1B-3 完成) `oidc_provider_handler_test.go` 用 `httptest` 覆盖：Discovery (disabled 404 / enabled 200 + issuer 断言)、JWKS (200 + 不泄露私钥)、Token (disabled 404、authorization_code 成功签发 access/refresh/id + `Cache-Control: no-store`、refresh 轮转、invalid_client→401、unsupported_grant_type→400)、UserInfo (Bearer 返回 name/email、缺 token→401 + `WWW-Authenticate`、非法 token→401 `error="invalid_token"`)、Authorize (无 session 跳 /login、非法 redirect_uri→400 JSON invalid_request)。全部通过

## 8. Admin Settings Plumbing

- [x] 8.1 (Stage 1B-2 完成，设计偏差) 出于风险考量，未把 8 个 key 塞进 3700 行的 `setting_handler.go` `SystemSettings` 巨型结构，而是新增**独立的** admin 设置端点 `backend/internal/handler/admin/oidc_provider_settings_handler.go` (`GET/PUT /api/v1/admin/oidc/settings`)，底层走 `OidcProviderService.GetProviderSettings/UpdateProviderSettings`，集中处理 8 个 key 的默认值/类型与 `ValidateOidcIssuerURL` 校验钩子，前端在 SettingsView 独立区块对接。
- [x] 8.2 (Stage 1B-2 完成) issuer_url format validator 已在 `UpdateProviderSettings` 接入：违规返回 issuer URL sentinel → handler 映射 HTTP 400；TTL 非正整数返回 `ErrOidcProviderInvalidTTL` → 400
- [x] 8.3 (Stage 1B-2 完成) signing-keys admin handlers (`oidc_signing_key_handler.go`): `GET /signing-keys` (list with `is_active`/`created_at`/`retired_at`/`removable`)、`POST /signing-keys/rotate`、`DELETE /signing-keys/:kid` (active key 删除返回 409 Conflict)
- [x] 8.4 (Stage 1B-3 完成) admin 操作审计：新增 `auditOidcAdmin` helper (结构化 slog `audit=true` + `component=audit.oidc_provider`，沿用 setting/user handler 既有约定，OIDC 低频管理操作无需独立审计表)，覆盖 client create/update/delete/reset-secret、signing-key rotate/delete、settings update，记录 operator_id/role + 关键字段

## 9. Frontend — Consent Page

- [x] 9.1 (Stage 1B-3 完成) Add route `/oauth/consent` in `frontend/src/router/index.ts` with `requiresAuth: true`；ConsentView 内对 401/login_required 自行 `redirect=<fullPath>` 跳登录（与后端 `/oauth/consent?consent=` 跳转路径一致）
- [x] 9.2 (Stage 1B-3 完成) Create `frontend/src/views/oidc/ConsentView.vue`：读 `consent` query 短期签名 token，调 `GET /oidc/consent?consent=<>`（独立 axios 实例 `withCredentials`，根路径 raw JSON）拿 client_name + scopes，渲染 Allow/Deny
- [x] 9.3 (Stage 1B-3 完成) i18n 硬编码 scope 文案：`oidc.consent.scopes.*` 在 zh.ts/en.ts，敏感 scope (balance/apikey) 红点+红字+顶部警示横幅
- [x] 9.4 (Stage 1B-3 完成) Allow → `POST /oidc/consent {consent, action:"allow"}` → `window.location.assign(redirect)`
- [x] 9.5 (Stage 1B-3 完成) Deny → `POST /oidc/consent {consent, action:"deny"}` → 后端返回 `redirect_uri?error=access_denied`，前端同样 `window.location.assign`
- [x] 9.6 (Stage 1B-3 完成) `ConsentView.spec.ts` (Vitest + @vue/test-utils)：scope 标题/描述渲染、Allow/Deny 各自调用 `submitConsentDecision('allow'|'deny')` 并整页 `location.assign(redirect)`、请求含 `sub2api:balance` 时红色警示横幅可见（无敏感 scope 时不渲染）、401 引导登录并保留 `redirect` 回跳。6 个用例全过

## 10. Frontend — Admin Pages

- [x] 10.1 (Stage 1B-3 完成) Create `frontend/src/api/admin/oidcClients.ts` wrapping the 6 admin endpoints + settings + signing-keys，含 6 scope 常量与 `isSensitiveScope`
- [x] 10.2 (Stage 1B-3 完成) Create `frontend/src/views/admin/OidcClientsView.vue`：`AppLayout`+`TablePageLayout`+`DataTable`，Create modal + Edit/Delete/ResetSecret 行操作
- [x] 10.3 (Stage 1B-3 完成) Create/Edit form：client_name、动态 redirect_uris、6-scope 复选、consent_required/enabled toggle
- [x] 10.4 (Stage 1B-3 完成) 勾选 balance/apikey 显示红色警示，保存前弹 checkbox 门控的确认 modal
- [x] 10.5 (Stage 1B-3 完成) Create 成功一次性展示明文 secret，复制按钮 + 不可关闭红色横幅（`:close-on-escape=false`+no-op close）
- [x] 10.6 (Stage 1B-3 完成) ResetSecret 用 ConfirmDialog 解释旧 secret 立即失效、旧 token 到期前仍可用；成功复用 one-time secret reveal
- [x] 10.7 (Stage 1B-3 完成) Add route `/admin/oidc-clients` + 侧边栏 `nav.oidcClients` 导航项 (key icon)
- [x] 10.8 (Stage 1B-3 完成) OIDC Provider 作为 SettingsView 新 tab：自包含子组件 `OidcProviderSettingsSection.vue` 对接独立 `/admin/oidc/settings`；issuer_url 内联格式校验；enable toggle 前置 ConfirmDialog；全局保存按钮对该 tab 隐藏

## 11. Documentation & Operator Materials

- [x] 11.1 (Stage 1B-3 完成) `OidcProviderSettingsSection.vue` 新增"第三方接入说明"区块：由 issuer_url 推导 discovery / jwks 端点、列出 6 个支持的 scope、redirect_uri 精确匹配规则、PKCE S256 强制要求（i18n `oidc.admin.help.*`）
- [x] 11.2 (Stage 1B-3 完成) 确认无需新增 YAML 选项 —— 8 个 OIDC 设置全部 DB 存储；该事实已在 README 注明（无须改 `deploy/config.example.yaml`）
- [x] 11.3 (Stage 1B-3 完成) README `Features` 新增 "OIDC Provider (SSO)" 条目，说明可作为 OIDC Provider、全部在 admin 后台配置、默认关闭（端点 404）

## 12. Integration & Conformance Verification

- [x] 12.1 (Stage 1B-3 完成) `internal/handler/oidc_integration_test.go` 用 gin engine 驱动完整流程：签发 SSO cookie → GET `/oidc/authorize` (consent_required=false) 302 回跳 `redirect_uri?code=&state=` → POST `/oidc/token` (authorization_code + PKCE S256) 拿 access/refresh/id → 拉 `/.well-known/jwks.json` 手工用 stdlib (`crypto/rsa`+`math/big`+`golang-jwt/jwt v5`) 重建 RSA 公钥并按 kid 验 RS256 签名、断言 iss/aud/sub/nonce → refresh 轮转出新 token + 新可验证 id token → UserInfo Bearer 取 sub/name/email → 旧 refresh 复用触发 family 吊销返回 `invalid_grant`。(go-oidc 未 vendored，按 task 指引手写验签。) 通过
- [ ] 12.2 Manual conformance smoke test using `oidc-client-tools` or `appauth` against a staging deployment; record results in PR description
- [ ] 12.3 Manual test: rotate signing key while a client holds a valid ID Token; verify that token still verifies for 7 days and stops verifying after the retired-timestamp setting is manually backdated past the grace window
- [ ] 12.4 Manual test: simulate refresh-token reuse; verify family revocation behavior and the security log line is emitted
- [ ] 12.5 Manual test: cross-subdomain SSO — set `sso_cookie_domain=.sub2api.com` (or your test domain) and verify a fresh `/oidc/authorize` from a sibling subdomain finds the SSO session

## 13. Release Sequencing

- [ ] 13.1 Open PR with all backend + frontend + migration changes; CI green
- [ ] 13.2 Merge with `oidc_provider.enabled=false` default; verify endpoints return 404 in production
- [ ] 13.3 In a staging admin UI: configure `issuer_url`, generate signing key (auto on first enable toggle), create one test OIDC client, run end-to-end with a sample RP application
- [ ] 13.4 Flip `enabled=true` in production after staging passes
- [ ] 13.5 Document the rollback procedure in PR description (toggle `enabled=false`; no data loss)
