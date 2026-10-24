# Sub2API OAuth Provider 改造方案

状态：最终方案（Provider + dsh 桌面客户端）

目标：支持 `dsh-sub2api-sync` 通过 OAuth 2.1 Authorization Code + PKCE 登录 Sub2API，并使用当前用户身份访问已有的用户、分组和 API Key 能力。

## 1. 结论

资源接口不需要重复创建为：

```text
/api/v1/oauth/v1/me
/api/v1/oauth/v1/me/groups
/api/v1/oauth/v1/me/keys
```

现有接口可以复用，但不能直接原样暴露给 OAuth Token。当前接口使用面板 JWT 鉴权，并且 API Key 查询响应、资源范围和写操作语义是为第一方 Web 面板设计的。

本方案采用以下边界：

- 新增 OAuth Provider 专用接口：授权、Token、撤销和能力探测；授权关系查询作为可选辅助接口。
- 复用现有 `/api/v1/auth/me`、`/api/v1/groups/available`、`/api/v1/keys` 的 handler/service 业务逻辑。
- 新增 OAuth-aware 鉴权适配层，使资源路由同时支持面板 JWT 和 OAuth access token。
- OAuth access token 通过 scope、client_id 和 auth kind 进入资源处理；不信任客户端传入的 client_id 过滤参数。
- 对 API Key 增加 OAuth 视图：列表/详情脱敏、明文仅走受控 reveal；来源隔离（按 client 过滤 Key）作为后续演进，第一版不区分来源、不改动 `api_keys` 表结构（见 §6）。

## 2. 接口分工

### 2.1 新增 OAuth Provider 接口

```text
GET  /api/v1/oauth/.well-known
GET  /api/v1/oauth/authorize
GET  /api/v1/oauth/authorize/transaction/:id  # 授权页读取短期事务展示信息
POST /api/v1/oauth/authorize/approve
POST /api/v1/oauth/token
POST /api/v1/oauth/revoke
GET  /api/v1/oauth/grants/current       # 可选，仅用于已登录用户或当前事务查询
```

这些接口没有现有等价物，必须新增：

- `/authorize`：校验 client、redirect_uri、scope、state 和 PKCE，并展示用户确认页。
- `/authorize/approve`：消费浏览器授权事务，批准或拒绝本次授权；不能仅使用 GET 完成批准。
- `/token`：authorization_code 和 refresh_token grant；refresh token 必须轮换。
- `/revoke`：撤销 access token、refresh token 或授权关系。
- `.well-known`：向插件返回真实 endpoint 地址和支持的 grant、scope、PKCE 能力。
- `/grants/current`：可选辅助接口；不能在没有用户身份时仅凭 client_id 判断授权状态。

### 2.2 复用现有资源接口

| OAuth 资源能力 | 复用接口 | OAuth 要求 |
|---|---|---|
| 用户资料 | `GET /api/v1/auth/me` | `openid` 或 `profile` |
| 可用分组 | `GET /api/v1/groups/available` | `groups:read` |
| Key 列表 | `GET /api/v1/keys` | `keys:read`，返回当前用户全部 Key 元数据（第一版不做来源过滤，见 §6） |
| 创建 Key | `POST /api/v1/keys` | `keys:create`，禁止自定义 Key 和未允许字段 |
| 删除/撤销 Key | `DELETE /api/v1/keys/:id` | `keys:revoke`，可撤销当前用户的 Key（第一版不做来源过滤，见 §6） |

`PUT /api/v1/keys/:id` 第一阶段不对 dsh 开放。后续如有需要，新增独立 `keys:update` scope 和字段白名单。

`POST /api/v1/keys/:id/reveal` 是新增的受控动作，不是完整资源 API。它允许展示当前用户的 Key，必须经过 `keys:read` scope、归属和审计校验；第一版不要求短时二次授权（见 §6）。

## 3. 为什么不能直接把现有接口当作 OAuth API

### `/api/v1/auth/me`

可以复用 handler，但当前路由只挂载 JWT middleware。需要增加 OAuth access token 解析，并将用户身份写入同一 `AuthSubject` 上下文。

### `/api/v1/groups/available`

可以复用 service 和 DTO，但必须增加 `groups:read` scope 校验。该接口返回的是当前用户可绑定的分组，正好符合 dsh 的读取需求。

### `/api/v1/keys`

不能无改动直接复用：

- 当前列表语义是返回用户全部 API Key；dsh 只能看到自己创建的 Key。
- 当前 DTO 含有 `key` 字段，不能把历史密钥明文批量返回。
- 当前创建请求支持自定义 Key、IP 黑白名单等第一方字段，OAuth Client 不应默认拥有这些能力。
- 当前删除接口只校验用户归属；第一版 OAuth 直接复用当前用户的 Key 权限，并额外要求 `keys:revoke` scope。

因此 `/keys` 的底层 service 可以复用，handler 只需要根据 OAuth auth kind 做 scope 校验和掩码投影。

## 4. OAuth Token 与现有 JWT 的关系

OAuth access token 不建议直接复用当前面板 JWT：

- 面板 JWT 没有 client、scope、grant 和 token family 语义。
- 现有 refresh 接口是第一方登录会话刷新，不支持 OAuth refresh rotation。
- OAuth 撤销需要按 Client、授权关系和 token family 精确撤销。

新增 `OAuthAccessTokenMiddleware`，或在现有 JWT middleware 外增加组合认证层：

```text
Authorization: Bearer <token>
  ├─ 能解析为面板 JWT：保持现有行为
  └─ 能解析为 OAuth token：加载 user_id、client_id、scope、token_id
```

OAuth token 解析成功后，应将以下信息写入上下文：

```text
auth_kind = oauth
user_id
oauth_client_id
oauth_token_id
oauth_scopes
```

资源 handler 只通过上下文读取这些值，不读取请求参数中的 client_id。

## 5. Scope 与 dsh 默认申请

第一阶段支持：

```text
openid
profile
groups:read
keys:read
keys:create
keys:revoke
```

dsh 默认申请：

```text
openid profile groups:read keys:read keys:create
```

启用 `deleteOrphanKeys` 时追加 `keys:revoke`。用户确认页必须按实际 scope 展示权限，不能只展示应用的固定描述。

### 5.1 dsh Client 注册

默认注册一个公开桌面 Client：

```text
client_id: dsh-sub2api-sync
client_type: public
pkce_required: true
```

loopback redirect URI 使用固定路径、动态端口：

```text
http://127.0.0.1:<dynamic-port>/oauth/callback
```

只允许 `127.0.0.1` 或 `::1`，不允许 `0.0.0.0`、局域网地址、任意通配域名或任意路径。动态端口的安全性由 PKCE、state、随机 attempt 和本机回调监听共同保证。

`authorize` 至少要求以下参数：

```text
response_type=code
client_id
redirect_uri
scope
state
code_challenge
code_challenge_method=S256
```

`token` 的 authorization code 请求必须再次提交完全相同的 `redirect_uri`、`client_id` 和 `code_verifier`。任何不匹配都返回 `invalid_grant`，不透露具体失败原因。

## 6. 数据模型

第一版只在 PostgreSQL 持久化 Client 和授权关系；授权码、access token、refresh token 全部使用 Redis：

```text
oauth_clients
oauth_grants
```

Redis Key：

```text
oauth:transaction:<id>  TTL 10 分钟
oauth:code:<sha256>     TTL 60 秒，GETDEL 单次消费
oauth:access:<sha256>   TTL 15 分钟
oauth:refresh:<sha256>  TTL 30 天，轮换时 GETDEL
```

Redis transaction 必须保存 client、redirect URI、scope、state、PKCE challenge、浏览器会话绑定和 attempt 绑定信息。

关键安全字段：

- Redis Key 使用随机 Token 的 SHA-256 作为索引，Value 保存最小必要的用户、Client、scope 和 refresh family 信息；
- dsh 是公开桌面 Client，不依赖共享 client secret，PKCE S256 是强制安全边界；
- authorization code 保存 redirect_uri、code_challenge 和 code_challenge_method；
- refresh token 保存 family_id、rotated_at、revoked_at；
- grant 保存用户、Client、scope 和撤销时间。

第一版不区分面板创建和 Client 创建的 API Key。OAuth Client 在 `keys:read`、`keys:create`、`keys:revoke` scope 允许范围内使用当前用户全部 API Key；列表响应仍只返回掩码，明文只在创建或 reveal 时返回。

## 7. `.well-known` 契约

建议响应中返回标准 OAuth endpoint 字段和 Sub2API 资源扩展：

```json
{
  "issuer": "https://sub2api.example.com/api/v1/oauth",
  "authorization_endpoint": "https://sub2api.example.com/api/v1/oauth/authorize",
  "token_endpoint": "https://sub2api.example.com/api/v1/oauth/token",
  "revocation_endpoint": "https://sub2api.example.com/api/v1/oauth/revoke",
  "userinfo_endpoint": "https://sub2api.example.com/api/v1/auth/me",
  "groups_endpoint": "https://sub2api.example.com/api/v1/groups/available",
  "keys_endpoint": "https://sub2api.example.com/api/v1/keys",
  "code_challenge_methods_supported": ["S256"],
  "grant_types_supported": ["authorization_code", "refresh_token"],
  "scopes_supported": ["openid", "profile", "groups:read", "keys:read", "keys:create", "keys:revoke"]
}
```

插件以 `.well-known` 返回的 endpoint 为准，不硬编码资源路径。这样将来如需迁移资源路径，不需要同步修改插件版本。

## 8. 代码改造位置

建议新增：

```text
backend/internal/handler/oauth_provider_handler.go
backend/internal/service/oauth_provider_service.go
backend/internal/service/oauth_token_service.go
backend/internal/server/middleware/oauth_access_token.go
backend/internal/server/routes/oauth_provider.go
backend/ent/schema/oauth_client.go
backend/ent/schema/oauth_authorization_code.go
backend/ent/schema/oauth_access_token.go
backend/ent/schema/oauth_refresh_token.go
backend/ent/schema/oauth_grant.go
backend/migrations/xxx_oauth_provider.sql
```

现有代码复用点：

- 用户身份和 Token 生成逻辑：`backend/internal/service/auth_service.go`；
- 分组权限和 API Key 创建：`backend/internal/service/api_key_service.go`；
- 用户资料：`backend/internal/handler/auth_handler.go`；
- 分组和 Key 路由：`backend/internal/server/routes/user.go`；
- 现有 OAuth 登录 pending session：仅参考 state、过期和 Cookie 处理，不直接复用为 authorization code 表。

## 9. 前端改造

宿主前端新增：

```text
frontend/src/views/oauth/AuthorizeView.vue
frontend/src/views/oauth/OAuthErrorView.vue
frontend/src/components/user/profile/ProfileConnectedAppsCard.vue
frontend/src/components/admin/settings/OAuthClientsCard.vue
```

页面职责：

- `/oauth/authorize`：登录、展示 dsh 信息和实际 scope、同意/拒绝；
- `/oauth/error`：处理非法 Client、redirect_uri、scope、state 和用户拒绝；
- 已连接的应用：`ProfileConnectedAppsCard` 卡片挂在 `/profile` 页（TOTP/Passkey 卡片旁），用户在个人资料页查看和撤销授权，不设独立路由；
- OAuth 客户端管理：`OAuthClientsCard` 卡片挂在管理端「系统设置 → 安全与认证」标签下，走 `/api/v1/admin/oauth-clients` CRUD（adminAuth + 审计）；第一版仅管理 public 客户端，PKCE 强制，confidential secret 轮换留待后续。

插件配置 iframe 不承担授权确认页，因为当前插件 UI 是管理员沙箱，不拥有用户会话和 OAuth redirect 能力。

## 10. 实施顺序

### Phase 1：Provider 基础

- Ent schema 和迁移；
- dsh public client 注册和 redirect URI 策略；
- `.well-known`、`authorize`、`token`、`revoke`；
- PKCE S256、state、authorization code 单次消费；
- refresh token rotation；
- 先用 curl 完成 Provider 流程测试。

### Phase 2：现有资源接口接入 OAuth

- 组合 JWT/OAuth 鉴权 middleware；
- `/auth/me` 增加 `openid/profile` scope；
- `/groups/available` 增加 `groups:read`；
- `/keys` 增加 OAuth 投影和字段白名单，不增加来源字段；
- 实现受控 reveal 和撤销。

### Phase 3：前端与插件联调

- consent/error 页面；
- connected apps 页面；
- dsh `/oauth/start`、`/oauth/callback`、`/oauth/status`；
- OAuth refresh 和 revoke；
- T25-T32 联调测试。

## 11. DeepSeek DSH 流程带来的补充设计

截图中的 DeepSeek 流程具有以下特征：

- 浏览器打开带有短期 `authorize_id` 的授权页面；
- 页面显示当前登录账号和“信任当前客户端”的风险提示；
- 用户点击登录/授权后，服务端完成授权事务；
- 成功页尝试通过 HTTPS App Link 或自定义协议唤起桌面应用；
- 桌面应用不依赖浏览器直接展示 Token，而是通过回调或状态查询取得结果。

这更接近“浏览器授权事务 + 桌面应用回传”的 OAuth 封装，不应把 `authorize_id` 当成 access token，也不能只靠它证明用户授权。Sub2API 应增加以下约束：

1. dsh 在 `/oauth/start` 生成 `state`、PKCE `code_verifier`，并通过授权 URL 发送 `code_challenge`。
2. Sub2API 为授权请求建立短期 transaction，绑定 `client_id`、精确 `redirect_uri`、scope、state、code_challenge、浏览器会话和过期时间。
3. `authorize_id`（如保留）只作为不可猜测的展示/查询标识，不能单独换 Token；查询还必须绑定浏览器 Cookie 或 dsh 的轮询凭据。
4. 授权页必须显示 Client 名称、来源、请求 scope、Key 操作范围和撤销方式。文案应明确“不会获得 Sub2API 密码、上游 OAuth Token 或其他用户数据”。
5. 用户批准必须通过 `POST /authorize/approve` 完成，并使用 CSRF token；GET 只负责展示页面。
6. 成功后只向 dsh 的已登记 redirect URI 返回一次性 authorization code 和原始 state，绝不在 URL、fragment、页面 HTML 或日志中返回 access token/refresh token。
7. 对桌面应用优先使用 `http://127.0.0.1:<port>/oauth/callback` loopback 回调并强制 PKCE；自定义 scheme 仅在 dsh 明确注册并能防止其他本地程序抢占时启用。
8. 回调不可用时，dsh 可以用一次性 transaction handle 调用 `/oauth/status` 轮询，但 status 凭据必须短期、单用途并绑定 attempt，不能替代 Token endpoint 的 code_verifier 校验。
9. 成功页提供“返回应用/关闭页面”的兜底，不依赖浏览器一定能唤起桌面应用。

### 11.1 对抓包顺序的逐步解读

用户提供的序列可以抽象为：

```text
本地 dsh-app RPC startSignIn
  -> 创建本地 attempt 和 callbackOrigin
  -> 浏览器打开带 authorize_id 的 Web 授权页
  -> Web 登录态调用 auth_approve(authorize_id)
  -> 服务端返回 loopback callback_url(code, state)
  -> 桌面应用接收 /oauth/callback
  -> 桌面应用再向自己的 Token 接口兑换会话凭据
```

其中：

- `account/startSignIn` 是桌面应用内部 RPC，不是 OAuth Provider 公共接口。Sub2API 不需要复制 `dsh-app://` 协议，只需要为插件提供本地 start/callback/status 生命周期。
- `auth_approve` 使用的是 DeepSeek Web 登录会话；它返回的是一次性 `code` 和 `state`，不是最终访问 Token。这与 Authorization Code 流程一致。
- `callback_url` 使用 `127.0.0.1` 回环地址，说明桌面应用采用 RFC 8252 类 Native App 回调。Sub2API 应优先采用相同模式。
- `settings`、`users/current`、`check_device` 是 DeepSeek 登录后的 Web 客户端初始化或设备检查接口，不应被复制到 Sub2API OAuth 协议中。Sub2API 只需在 Token 兑换后调用自身的 `/auth/me`、分组和 Key 资源接口。
- 抓包没有证明 DeepSeek 使用了 PKCE，也没有展示桌面应用最终的 Token exchange 请求。因此 Sub2API 不能照搬其内部接口，必须明确强制 PKCE S256。
- `x-client-*`、`x-device-id` 和 User-Agent 只能作为审计或设备元数据，不能作为客户端身份或授权凭据。Client 身份必须由 `client_id`、已登记 redirect URI 和 PKCE 共同确定。

### 11.2 重新设计后的 dsh 对接流程

不再让插件先依赖一个共享 confidential secret，改为公开桌面 Client：

```text
dsh 本地生成 attempt、state、code_verifier 和随机 loopback 端口
  -> 打开 Sub2API /oauth/authorize?client_id=dsh&redirect_uri=...&code_challenge=...
  -> 用户登录并在 Sub2API 页面批准 scope
  -> Sub2API 重定向到 http://127.0.0.1:<port>/oauth/callback?code=...&state=...
  -> 插件校验 state，并用 code_verifier POST /oauth/token
  -> 插件持久化 access_token/refresh_token
  -> 插件使用现有 /auth/me、/groups/available、/keys 接口同步模型配置
```

如果本地回环监听失败，插件可以退化为一次性 transaction polling，但 polling 只用于等待授权结果，最终仍必须通过 `/oauth/token` + `code_verifier` 兑换 Token。

建议同时提供标准发现地址：

```text
GET /.well-known/oauth-authorization-server
```

保留 `/api/v1/oauth/.well-known` 作为 dsh 兼容别名。标准地址应返回 RFC 8414 风格字段，自定义资源地址继续放在扩展字段中。

标准发现地址是协议首选；插件启动时先请求标准地址，404 时再请求兼容别名。发现失败或返回不完整时，不显示 OAuth 登录入口。

## 12. 对当前五个接口方案的安全评估

原五个接口是控制面候选集合，但单独列出时还不完备：

```text
GET  /api/v1/oauth/.well-known
GET  /api/v1/oauth/authorize
POST /api/v1/oauth/token
POST /api/v1/oauth/revoke
GET  /api/v1/oauth/grants/current
```

必须补充：

```text
POST /api/v1/oauth/authorize/approve
```

此外：

- `grants/current` 不是 OAuth 标准必需接口；如果用户尚未登录，服务端无法仅凭 Client 判断“当前用户”。建议它只接受已登录面板会话，或改为查询当前 authorization transaction。
- 如果 access token 使用不可自包含的 opaque token，不需要 `introspect`；Sub2API 内部直接查 token 哈希即可。
- 如果未来需要第三方资源服务器独立验 Token，再考虑增加 `introspect`，并限制给受信任后端 Client。
- 资源访问继续使用现有 `/auth/me`、`/groups/available`、`/keys`，通过 OAuth-aware middleware 和 scope 控制，不新增 `/oauth/v1/me*` 重复 API。
- `.well-known` 返回的资源 endpoint 必须是最终部署地址，插件不得硬编码域名或路径。

因此，完整的第一版控制面应为：

```text
GET  /.well-known/oauth-authorization-server
GET  /api/v1/oauth/.well-known
GET  /api/v1/oauth/authorize
POST /api/v1/oauth/authorize/approve
POST /api/v1/oauth/token
POST /api/v1/oauth/revoke
```

以下接口不是授权码流程的硬依赖，可按产品需要实现：

```text
GET  /api/v1/oauth/grants/current
```

`/.well-known/oauth-authorization-server` 是标准发现地址，`/api/v1/oauth/.well-known` 是 dsh 兼容地址，二者都属于第一版必需能力。`grants/current` 只允许已登录面板会话或已绑定的短期授权事务调用。

再加上现有资源接口的 OAuth 鉴权适配和受限 Key 投影，才构成可供 dsh 使用的完整方案。

## 13. 最终决策

本方案不新增 `/api/v1/oauth/v1/me*` 资源族。最终采用：

```text
新 OAuth 控制面：/api/v1/oauth/*
现有资源接口：/api/v1/auth/me、/api/v1/groups/available、/api/v1/keys
OAuth-aware middleware + scope + client 来源过滤
```

这样可以最大限度复用现有业务代码，同时避免把面板 JWT、OAuth scope、API Key 来源和第三方权限混在一起。
