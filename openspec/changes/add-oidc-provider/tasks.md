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
- [ ] 3.3 (Stage 1B-2) Locate every successful login path and add `ssoSessionService.Issue` call: `auth_handler.go` Login, `auth_email_oauth.go` magic-link complete, `auth_oidc_oauth.go` complete, `auth_dingtalk_oauth.go` complete, `auth_wechat_oauth.go` complete, `auth_linuxdo_oauth.go` complete, registration auto-login path
- [ ] 3.4 (Stage 1B-2) Wire logout to call `Revoke` and emit `Set-Cookie: sub2api_sso=; Max-Age=0; Path=/`
- [x] 3.5 Implement `TouchLastSeen` with rate limiting (no DB write if last update was within the last 60s) and run from a goroutine so it never blocks `/oidc/authorize`
- [x] 3.6 Unit tests: Issue produces correct cookie attributes per setting combinations; Resolve handles missing/expired/revoked sessions correctly; Revoke ends both DB row and cookie

## 4. OIDC Client Service & Admin CRUD

- [x] 4.1 Create `backend/internal/service/oidc_client_service.go` with `Create(req) (client *OidcClient, plaintextSecret string, err error)`, `List(filters)`, `Get(id)`, `Update(id, patch)`, `Delete(id)`, `ResetSecret(id) (plaintextSecret string, err error)`, `Authenticate(ctx, clientID, presentedSecret) (*OidcClient, error)`
- [x] 4.2 Implement secret generation: 32 bytes from `crypto/rand`, base64url encode no padding; `client_id` = `"rp_" + base32(rand 16B without padding lowercase)`
- [x] 4.3 Implement `Authenticate` using `bcrypt.CompareHashAndPassword`; return distinct error sentinels for unknown client vs. wrong secret (handler converts both to `invalid_client` to avoid enumeration)
- [x] 4.4 Validate redirect_uris on Create/Update: must be non-empty array, every entry parseable URL with `https://` scheme (allow `http://localhost` for dev), no trailing slash mismatch (store exactly as provided)
- [x] 4.5 Validate allowed_scopes is subset of `["openid","profile","email","offline_access","sub2api:balance","sub2api:apikey"]`
- [x] 4.6 Implement cascade delete: Delete must remove all `oidc_consent`, `oidc_authorization_code`, `oidc_refresh_token` rows referencing `client_id` in the same transaction (本实现额外把 `oidc_access_token` 也纳入级联，符合决策 B1 的独立表方案)
- [ ] 4.7 (Stage 1B-2) Create `backend/internal/handler/admin/oidc_client_handler.go` with handlers for the 6 admin routes (List, Create, Get, Patch, Delete, ResetSecret)
- [ ] 4.8 (Stage 1B-2) Register routes under `/api/v1/admin/oidc/clients` group with admin auth middleware
- [x] 4.9 Unit tests for service: secret round-trip with bcrypt, redirect_uri validation, allowed_scopes subset enforcement, cascade delete, ResetSecret invalidates the old secret
- [ ] 4.10 (Stage 1B-2) Handler tests: Create returns plaintext secret only in creation response; subsequent Get omits it

## 5. OIDC Consent Service

- [x] 5.1 Create `backend/internal/service/oidc_consent_service.go` 暴露 `LoadGrantedScopes`、`Grant`、`Revoke`、`TouchLastUsed`、`IsCovered`。
- [x] 5.2 `IsCovered` 严格 superset 判断：空 requested → true；空 granted + 非空 requested → false；忽略顺序与重复元素。
- [x] 5.3 `Grant` upsert：行存在则 `granted_scopes = 旧 ∪ 新` 并刷新 `last_used_at`；行不存在则插入新行；`TouchLastUsed` 仅刷 `last_used_at` 不动 scope，供 authorize 命中 superset 时使用。
- [x] 5.4 单测 9 项全过 (`TestOidcConsent_*` + `TestUnionStrings_*`)：未命中 / 插入新行 / 增量并集 / 同一请求内部去重 / Revoke 行 + 二次 Revoke 报错 / TouchLastUsed 不存在行报错 / IsCovered 全场景 / unionStrings 顺序与去重。

## 6. OIDC Provider Core Service

- [ ] 6.1 Create `backend/internal/service/oidc_provider_service.go` with high-level methods: `HandleAuthorize(ctx, params) (*AuthorizeOutcome, error)`, `ExchangeCode(ctx, params) (*TokenResponse, error)`, `RefreshToken(ctx, params) (*TokenResponse, error)`, `RevokeFamily(ctx, familyID) error`, `BuildUserInfo(ctx, accessTokenValue) (claims map[string]any, err error)`
- [ ] 6.2 Implement authorize parameter validation per spec scenarios (response_type, scope subset, redirect_uri exact match, PKCE S256 required, prompt=login handling, client.enabled check)
- [ ] 6.3 Implement opaque code generation: 32 bytes from `crypto/rand`, base64url no padding; persist with code_ttl_seconds
- [ ] 6.4 Implement `ExchangeCode`: load code by hash-equality (exact match on the stored value), check unconsumed/unexpired, validate redirect_uri/PKCE, mark consumed atomically, generate access_token and (if offline_access) refresh_token, sign ID Token via `oidcSigningService.SignIDToken`
- [ ] 6.5 Implement access token storage: opaque tokens persist in a NEW table `oidc_access_token` mirroring `oidc_refresh_token` minus family fields (or extend the refresh token table with a `kind` column — choose during implementation per simpler path; document the choice in PR description)
- [ ] 6.6 Implement `RefreshToken`: load by token, ensure not revoked/expired, atomically mark revoked + insert new refresh in same family, support optional scope downgrade (must be subset)
- [ ] 6.7 Implement reuse detection: if the presented refresh token is already revoked, call `RevokeFamily` and return `invalid_grant`; emit a security log line `oidc.refresh_token.reuse_detected`
- [ ] 6.8 Implement claim assembly per Scope-to-Claim Mapping requirement (D8): switch by scope, never put balance/apikey_count in id_token
- [ ] 6.9 Implement `BuildUserInfo`: lookup access token row, load user, project claims based on token's stored scopes
- [ ] 6.10 Implement `acr` / `amr` derivation from session: read latest `pending_auth_session.totp_verified_at` for the user (or sso_session metadata if mfa flag is added — choose minimal-touch path)
- [ ] 6.11 Unit tests: each spec scenario in `oidc-provider/spec.md` (Token Endpoint requirement) maps to at least one test case in this service

## 7. OIDC HTTP Handlers

- [ ] 7.1 Create `backend/internal/handler/oidc_provider_handler.go` with: `Discovery`, `JWKS`, `Authorize`, `Token`, `UserInfo`
- [ ] 7.2 Implement `Discovery` returning JSON per the Discovery requirement; 404 when `oidc_provider.enabled=false`
- [ ] 7.3 Implement `JWKS` returning `oidcSigningService.JWKS()` result; 404 when disabled
- [ ] 7.4 Implement `Authorize` GET: parse query params, call provider service, on success either redirect to consent page or directly to `redirect_uri` with `code`; on error use the redirect-vs-JSON branching per spec
- [ ] 7.5 Implement `Token` POST: support both `client_secret_basic` (Basic auth header) and `client_secret_post` (form fields); set `Cache-Control: no-store` and `Pragma: no-cache` per spec
- [ ] 7.6 Implement `UserInfo` GET: parse Bearer header (case-insensitive scheme), call `BuildUserInfo`, on error return `WWW-Authenticate: Bearer error="invalid_token"` per spec
- [ ] 7.7 Create `backend/internal/handler/oidc_provider_consent_handler.go` with `ConsentGet`, `ConsentPost` (CSRF-protected)
- [ ] 7.8 Create `backend/internal/server/routes/oidc_provider.go` registering the 6 OIDC endpoints + `/.well-known/openid-configuration` + `/.well-known/jwks.json` at the root router (no API version prefix)
- [ ] 7.9 Add startup validation that panics if `oidc_provider.enabled=true` but `issuer_url` is empty
- [ ] 7.10 Handler-level tests using `httptest`: each scenario in the Authorize, Token, UserInfo requirements is covered with at least one assertion

## 8. Admin Settings Plumbing

- [~] 8.1 (Stage 1B-1 部分完成) 新增 `backend/internal/service/oidc_provider_settings.go` 集中声明 8 个 `oidc_provider.*` setting key 常量、默认值、`ValidateOidcIssuerURL` (前缀/末尾斜杠/?#) 严格校验函数、`AllowedOidcProviderScopes` 与 `ValidateOidcProviderScopeSubset`。下一阶段 (Stage 1B-2) 仍需修改 `backend/internal/handler/admin/setting_handler.go` 把这 8 个 key 注册进 admin 设置 UI 的元数据 (默认值/类型/分组)，并接入 ValidateOidcIssuerURL 作为 setter 校验钩子。
- [ ] 8.2 (Stage 1B-2) Implement issuer_url format validator (must start with `https://`, must not end with `/`, must not contain `?` or `#`); return HTTP 400 with explicit message on violation — service 层校验函数已就绪，剩 handler 接入。
- [ ] 8.3 (Stage 1B-2) Implement signing-keys admin handlers: `POST /api/v1/admin/oidc/signing-keys/rotate`, `DELETE /api/v1/admin/oidc/signing-keys/:kid`, `GET /api/v1/admin/oidc/signing-keys` (list with `is_active`, `created_at`, `retired_at`, `removable` flags)
- [ ] 8.4 (Stage 1B-2) Add audit log entries for each admin operation: client create/update/delete/reset-secret, signing-key rotate/delete, settings change

## 9. Frontend — Consent Page

- [ ] 9.1 Add route `/oidc/consent` in `frontend/src/router/index.ts` with `requiresAuth: true`; if user not signed in, router auth guard redirects to login with `next` preserved
- [ ] 9.2 Create `frontend/src/views/oidc/ConsentView.vue`: read `session` query param, call `GET /oidc/consent?session=<>` to fetch client_name + requested scopes + scope-description map, render Allow/Deny buttons
- [ ] 9.3 Hard-code human-readable scope descriptions in i18n locale: `openid` (基础身份), `profile` (用户名), `email` (邮箱地址), `offline_access` (离线访问 / refresh token), `sub2api:balance` (**红色警示**：读取账户余额与累计充值), `sub2api:apikey` (读取已创建 API Key 数量，不会读取 key 内容)
- [ ] 9.4 Submit Allow → `POST /oidc/consent` with CSRF token + session token; on 200 follow `Location` redirect to `redirect_uri`
- [ ] 9.5 Submit Deny → `POST /oidc/consent` with `decision=deny`; backend returns redirect to `redirect_uri?error=access_denied`
- [ ] 9.6 Component tests using Vitest + Testing Library: scope descriptions render, Allow and Deny call the correct endpoints, red warning visible when `sub2api:balance` is requested

## 10. Frontend — Admin Pages

- [ ] 10.1 Create `frontend/src/api/oidcClients.ts` wrapping the 6 admin endpoints
- [ ] 10.2 Create `frontend/src/views/admin/OidcClientsView.vue` with: client list table (columns: name, client_id, allowed_scopes, redirect_uris count, enabled, created_at), Create button opening a drawer/modal, Edit/Delete/ResetSecret row actions
- [ ] 10.3 In Create/Edit form: `client_name`, dynamic redirect_uris input (add/remove), `allowed_scopes` as multi-select with the 6 valid scopes, `consent_required` toggle, `enabled` toggle
- [ ] 10.4 When user toggles on `sub2api:balance` or `sub2api:apikey` in `allowed_scopes`, show inline red warning text and require a confirm-modal with checkbox "我确认允许此客户端读取敏感信息" before save
- [ ] 10.5 Create-success page shows the plaintext `client_secret` once with a copy-to-clipboard button and a non-dismissable red banner "此 secret 不会再次显示，请立即复制保存"
- [ ] 10.6 ResetSecret action shows confirm dialog explaining all running tokens of that client will keep working until they expire, but the old secret stops working immediately; on success show the same one-time secret reveal page
- [ ] 10.7 Add route `/admin/oidc-clients` and link it in the admin sidebar navigation
- [ ] 10.8 Add a new section "OIDC Provider" in `frontend/src/views/admin/SettingsView.vue` with form fields for the 8 `oidc_provider.*` settings; for `issuer_url` show inline format hint and validation error; for `enabled` show a confirmation dialog before turning on

## 11. Documentation & Operator Materials

- [ ] 11.1 Add a help section inside the admin OIDC Provider page describing 3rd-party integration: discovery URL, supported scopes, redirect_uri exact-match rule, PKCE requirement
- [ ] 11.2 Update `deploy/config.example.yaml` if any new YAML-level option is required (likely none — all settings are DB-stored; document this fact in a README touch)
- [ ] 11.3 Add a section to project README briefly noting "sub2api can act as an OIDC provider" with a pointer to the admin docs

## 12. Integration & Conformance Verification

- [ ] 12.1 Add an integration test under `backend/internal/service/` (or a new `backend/test/integration` folder if convention exists) that boots an in-process gin server, drives a complete Authorization Code + PKCE + Refresh Token flow, and uses `github.com/coreos/go-oidc/v3` (or equivalent already-vendored library; if not vendored, do JWKS + RS256 verification by hand using stdlib) to validate the ID Token against the JWKS endpoint
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
